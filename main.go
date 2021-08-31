package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/containers/image/docker"
	"github.com/containers/image/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	RegistryURL, RegistryUserName, RegistryPassword string
)

func init() {
	RegistryURL = os.Getenv("REGISTRY")
	RegistryUserName = os.Getenv("REGISTRY_USERNAME")
	RegistryPassword = os.Getenv("REGISTRY_PASSWORD")
}

type Exporter struct {
	Logger  zerolog.Logger
	Client  *http.Client
	Request *http.Request
}

type Image struct {
	ImageWithTag   string
	TimeOfCreation time.Time
	Size           int
}

func main() {
	exporter := NewExporter()
	exporter.Logger.Info().Interface("starting", time.Now())
	ctx := context.Background()
	r := exporter.getRegistryImages(RegistryURL)
	var Images []Image
	for _, s := range r.Repositories {
		allTags := exporter.getTags(RegistryURL, s)
		for _, tag := range allTags {
			timeOfCreation, err := exporter.getDateOfCreation(RegistryURL, s, tag)
			if err != nil {
				exporter.Logger.Err(err).Msg("error getting tags")
			}
			var pp Image
			pp.ImageWithTag = RegistryURL + "/" + s + ":" + tag
			pp.TimeOfCreation = timeOfCreation
			pp.Size = exporter.getSize(ctx, pp.ImageWithTag)
			Images = append(Images, pp)
		}
	}
	cwd, _ := os.Getwd()
	SaveJsonFile(Images, path.Join(cwd, RegistryURL+"_reports.json"))
	exporter.Logger.Info().Interface("ending", time.Now())
}

func SaveJsonFile(v interface{}, path string) {
	fo, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer fo.Close()
	e := json.NewEncoder(fo)
	if err := e.Encode(v); err != nil {
		panic(err)
	}
}

func NewExporter() *Exporter {
	fs, _ := os.Create("/var/logs/registry_reports.log")
	log.Logger = log.With().Caller().Logger().Output(fs)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Info().Msg("starting exporter")
	req, _ := http.NewRequest("GET", "", nil)
	req.SetBasicAuth(RegistryUserName, RegistryPassword)
	client := &http.Client{
		Timeout: time.Second * 10,
	}
	return &Exporter{Logger: log.Logger, Request: req, Client: client}
}

func (exporter *Exporter) getSize(ctx context.Context, imageWithTag string) int {
	err := docker.CheckAuth(ctx, &types.SystemContext{}, RegistryUserName, RegistryPassword, strings.Split(imageWithTag, "/")[0])
	if err != nil {
		exporter.Logger.Err(err).Msg("error authenticating to docker registry")
		return -1
	}
	ref, err := docker.ParseReference("//" + imageWithTag)
	if err != nil {
		exporter.Logger.Err(err).Msgf("error parsing docker image %s", imageWithTag)
		return -1
	}

	img, err := ref.NewImage(ctx, nil)
	if err != nil {
		exporter.Logger.Err(err).Msgf("error referring the image %s", imageWithTag)
		return -1
	}
	defer img.Close()

	b, _, err := img.Manifest(ctx)
	if err != nil {
		exporter.Logger.Err(err).Msgf("error getting mnifest for the image %s", imageWithTag)
		return -1
	}

	var m ManifestConfig
	err = json.Unmarshal(b, &m)
	if err != nil {
		exporter.Logger.Err(err).Msg("error unmashalling the manifest json")
	}

	result := m.Config.Size
	for _, x := range m.Layers {

		result = result + x.Size

	}

	return result
}

type Repositories struct {
	Repositories []string `json:"repositories"`
}

func (exporter Exporter) getRegistryImages(registryUrl string) Repositories {
	apiUrl := "https://" + registryUrl + "/v2/_catalog"
	parsedAPIUrl, _ := url.Parse(apiUrl)
	exporter.Request.URL = parsedAPIUrl
	exporter.Request.Method = "GET"
	response, err := exporter.Client.Do(exporter.Request)
	if err != nil {
		exporter.Logger.Err(err).Msg("error calling rest aned point")
	}
	defer response.Body.Close()
	fmt.Println("getting registry images")
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		exporter.Logger.Err(err).Msg("error calling rest aned point")
	}

	var dt Repositories
	json.Unmarshal(body, &dt)
	return dt
}

type Tags struct {
	Tags []string `json:"tags"`
}

func (exporter Exporter) getTags(registryUrl, repository string) []string {
	apiUrl := "https://" + registryUrl + "/v2/" + repository + "/tags/list"
	parsedAPIUrl, _ := url.Parse(apiUrl)
	exporter.Request.URL = parsedAPIUrl
	exporter.Request.Method = "GET"
	response, err := exporter.Client.Do(exporter.Request)
	if err != nil {
		exporter.Logger.Err(err).Msg("error calling rest aned point")
	}
	defer response.Body.Close()
	fmt.Println("getting tags for ", repository)
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		exporter.Logger.Err(err).Msg("error calling rest endd point")
	}
	var dt Tags
	json.Unmarshal(body, &dt)
	return dt.Tags

}

type Manifest struct {
	FsLayers []struct {
		BlobSum string `json:"blobSum"`
	} `json:"fsLayers"`
	History []struct {
		V1Compatibility string `json:"v1Compatibility"`
	} `json:"history"`
}

type ManifestCreatedDate struct {
	Created string `json:"created"`
}

func (exporter Exporter) getDateOfCreation(registryUrl, repository, tag string) (time.Time, error) {
	apiUrl := "https://" + registryUrl + "/v2/" + repository + "/manifests/" + tag
	parsedAPIUrl, _ := url.Parse(apiUrl)
	exporter.Request.URL = parsedAPIUrl
	exporter.Request.Method = "GET"
	response, err := exporter.Client.Do(exporter.Request)
	if err != nil {
		exporter.Logger.Err(err).Msg("error calling rest aned point")
	}
	defer response.Body.Close()
	fmt.Printf("getting date of creation for %s %s \n", repository, tag)
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		exporter.Logger.Err(err).Msg("error calling rest aned point")
	}
	var dt Manifest
	json.Unmarshal(body, &dt)
	if len(dt.History) > 0 {
		x := (dt.History)[0]
		var m ManifestCreatedDate
		json.Unmarshal([]byte(x.V1Compatibility), &m)
		t, _ := time.Parse(time.RFC3339, m.Created)
		return t, nil
	} else {

		return time.Now(), fmt.Errorf("no tags found")
	}

}

type ManifestConfig struct {
	Config struct {
		Size int `json:"size"`
	} `json:"config"`
	Layers []struct {
		Size int `json:"size"`
	} `json:"layers"`
}
