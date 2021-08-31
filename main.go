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
	registryURLPrefix := (strings.Split(os.Getenv("REGISTRY_HOST"), "://"))[0]
	registryURLSuffix := (strings.Split(os.Getenv("REGISTRY_HOST"), "://"))[1]
	if registryURLPrefix == "https" {
		RegistryURL = os.Getenv("REGISTRY_HOST")
	} else {
		RegistryURL = registryURLSuffix
	}
	RegistryUserName = os.Getenv("REGISTRY_USERNAME")
	RegistryPassword = os.Getenv("REGISTRY_PASSWORD")
}

type Exporter struct {
	Logger  zerolog.Logger
	Client  *http.Client
	Request *http.Request
}

type Image struct {
	ImageNameWithTag string
	TimeOfCreation   time.Time
	Size             int
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
			var i Image
			i.ImageNameWithTag = strings.ReplaceAll(RegistryURL, "https://", "") + "/" + s + ":" + tag
			i.TimeOfCreation = timeOfCreation
			i.Size = exporter.getSize(ctx, i.ImageNameWithTag)
			Images = append(Images, i)
		}
	}
	cwd, _ := os.Getwd()
	SaveJsonFile(Images, path.Join(cwd, strings.ReplaceAll(RegistryURL, "https://", "")+"_reports.json"))
	exporter.Logger.Info().Interface("ending exporting", time.Now())
}

func SaveJsonFile(v interface{}, path string) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	e := json.NewEncoder(f)
	if err := e.Encode(v); err != nil {
		panic(err)
	}
}

func NewExporter() *Exporter {
	fs, _ := os.Create("registry_reports.log")
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

func (exporter *Exporter) getSize(ctx context.Context, ImageNameWithTag string) int {

	err := docker.CheckAuth(ctx, &types.SystemContext{}, RegistryUserName, RegistryPassword, strings.Split(ImageNameWithTag, "/")[0])
	if err != nil {
		exporter.Logger.Err(err).Msg("error authenticating to docker registry")
		return -1
	}
	ref, err := docker.ParseReference("//" + ImageNameWithTag)
	if err != nil {
		exporter.Logger.Err(err).Msgf("error parsing docker image %s", ImageNameWithTag)
		return -1
	}

	img, err := ref.NewImage(ctx, nil)
	if err != nil {
		exporter.Logger.Err(err).Msgf("error referring the image %s", ImageNameWithTag)
		return -1
	}
	defer img.Close()

	b, _, err := img.Manifest(ctx)
	if err != nil {
		exporter.Logger.Err(err).Msgf("error getting mnifest for the image %s", ImageNameWithTag)
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
	apiUrl := registryUrl + "/v2/_catalog"
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
		exporter.Logger.Err(err).Msg("error calling rest end point")
	}

	var repos Repositories
	json.Unmarshal(body, &repos)
	return repos
}

type Tags struct {
	Tags []string `json:"tags"`
}

func (exporter Exporter) getTags(registryUrl, repository string) []string {
	apiUrl := registryUrl + "/v2/" + repository + "/tags/list"
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
	var t Tags
	json.Unmarshal(body, &t)
	return t.Tags

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
	apiUrl := registryUrl + "/v2/" + repository + "/manifests/" + tag
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
	var mf Manifest
	json.Unmarshal(body, &mf)
	if len(mf.History) > 0 {
		layerZero := (mf.History)[0]
		var mc ManifestCreatedDate
		json.Unmarshal([]byte(layerZero.V1Compatibility), &mc)
		t, _ := time.Parse(time.RFC3339, mc.Created)
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
