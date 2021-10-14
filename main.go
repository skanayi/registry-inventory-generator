package main


import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sort"
	"strings"
	"sync"

	"net/http"
	"net/url"
	"os"
	"strconv"

	"time"

	"github.com/containers/image/docker"
	"github.com/containers/image/types"
	"github.com/go-resty/resty/v2"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/zenthangplus/goccm"
)

var (
	RegistryURL, RegistryUserName, RegistryPassword string
	RetentionDays                                   int
	NumberOfImages                                  int
	mx                                              sync.Mutex
	imageMap                                        map[string][]Tag
	exclusionList                                   map[string]bool
)

func init() {
	RegistryURL = os.Getenv("REGISTRY")
	RegistryUserName = os.Getenv("REGISTRY_USERNAME")
	RegistryPassword = os.Getenv("REGISTRY_PASSWORD")
	RetentionDays, _ = strconv.Atoi(os.Getenv("REGISTRY_RETENTION"))
	imageMap = make(map[string][]Tag)
	exclusionList = make(map[string]bool)
	if RegistryURL == "" || RegistryUserName == "" || RegistryPassword == "" {
		log.Error().Msgf("one of the mandatory env variable is empty")
		os.Exit(1)
	}

}

type Exporter struct {
	Logger zerolog.Logger
	Client *resty.Client
}
type Image struct {
	Tags []Tag
}

type Tag struct {
	Name           string
	TimeOfCreation time.Time
}

type Tags struct {
	Tags []string `json:"tags"`
}


func main() {
	ctx := context.Background()

	if RetentionDays <= 0 {
		fmt.Println("Retention days less than or equal to 0")
		os.Exit(1)

	}

	fmt.Println(time.Now())
	workers, _ := strconv.Atoi(os.Getenv("REGISTRY_WORKER"))
	exporter := NewExporter(ctx)
	exporter.Logger.Info().Interface("starting", time.Now())
	r := exporter.getRegistryImages(RegistryURL)
	c := goccm.New(workers)
	for _, s := range r.Repositories {
		c.Wait()
		go func(s string) {
			defer c.Done()
			_, exist := exclusionList[s]

			if !exist {

				exporter.getTagInfo(RegistryURL, s)
			}

		}(s)

	}
	c.WaitAllDone()

	var tempTagArrayOrig []Tag
	for key := range imageMap {
		for _, x := range imageMap[key] {

			var tempTag Tag
			tempTag.Name = key + ":" + x.Name
			tempTag.TimeOfCreation = x.TimeOfCreation
			tempTagArrayOrig = append(tempTagArrayOrig, tempTag)

		}

	}
	//print image details + dates of tag creation as json
	SaveJsonFile(tempTagArrayOrig, "imagemap_original.json")

	var tempTagArray []Tag
	for key := range imageMap {

		sort.SliceStable(imageMap[key], func(i, j int) bool {
			return imageMap[key][i].TimeOfCreation.Before(imageMap[key][j].TimeOfCreation)
		})
		if len(imageMap[key]) > RetentionDays {
			mapLength := len(imageMap[key])
			for _, x := range imageMap[key][:mapLength-RetentionDays] {
				var tempTag Tag
				tempTag.Name = key + ":" + x.Name
				tempTag.TimeOfCreation = x.TimeOfCreation
				tempTagArray = append(tempTagArray, tempTag)

			}

		}

	}

	//target list of images for deletion  by keeping recent RETENTION tags
	SaveJsonFile(tempTagArray, "imagemap_for_deletion.json")

	fmt.Println("time", time.Now())
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

func NewExporter(ctx context.Context) *Exporter {
	client := resty.New()
	client.RetryCount = 3
	client.SetTimeout(time.Duration(10 * time.Minute))
	client.SetBasicAuth(RegistryUserName, RegistryPassword)

	fs, _ := os.Create("/var/logs/exporter.log")
	log.Logger = log.With().Caller().Logger().Output(fs)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Info().Msg("starting exporter")

	err := docker.CheckAuth(ctx, &types.SystemContext{}, RegistryUserName, RegistryPassword, RegistryURL)
	if err != nil {
		log.Fatal().Msg("error logging in to registry")
	}
	return &Exporter{Logger: log.Logger, Client: client}

}


type Repositories struct {
	Repositories []string `json:"repositories"`
}

func (exporter Exporter) getRegistryImages(registryUrl string) Repositories {
	apiUrl := "http://" + registryUrl + "/v2/_catalog"

	resp, err := exporter.Client.R().Get(apiUrl)

	if err != nil {
		exporter.Logger.Err(err).Msgf("error occured")
	}

	var dt Repositories
	json.Unmarshal(resp.Body(), &dt)
	return dt

}


func (exporter Exporter) getTagInfo(registryUrl, repository string) {

	apiUrl := "http://" + registryUrl + "/v2/" + repository + "/tags/list"
	resp, err := exporter.Client.R().Get(apiUrl)

	if err != nil {
		fmt.Println("errarod error", err)
	}


	var dt Tags
	json.Unmarshal(resp.Body(), &dt)

	//fmt.Println("dt is", dt)

	for _, t := range dt.Tags {

		getDateOfCreation(registryUrl, repository, t)

	}

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

func getDateOfCreation(registryUrl, repository, tag string) {
	var tempTag Tag
	var tempTags []Tag

	req, _ := http.NewRequest("GET", "", nil)

	req.SetBasicAuth(RegistryUserName, RegistryPassword)
	client := &http.Client{
		Timeout: time.Second * 10,
	}
	apiUrl := "http://" + registryUrl + "/v2/" + repository + "/manifests/" + tag
	//fmt.Println("api url is", apiUrl)
	parsedAPIUrl, _ := url.Parse(apiUrl)
	req.URL = parsedAPIUrl
	response, err := client.Do(req)
	if err != nil {
		fmt.Println("errarod error", err)
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)

	if err != nil {
		fmt.Println("error calling rest aned point")
	}
	defer response.Body.Close()


	var dt Manifest
	json.Unmarshal(body, &dt)
	if len(dt.History) > 0 {
		x := (dt.History)[0]
		var m ManifestCreatedDate
		json.Unmarshal([]byte(x.V1Compatibility), &m)
		t, _ := time.Parse(time.RFC3339, m.Created)
		mx.Lock()
		tempTag.Name = tag
		tempTag.TimeOfCreation = t
		tempTags = append(tempTags, tempTag)
		registryKey := registryUrl + "/" + repository

		_, exist := imageMap[registryKey]
		if exist {

			imageMap[registryKey] = append(imageMap[registryKey], tempTags...)
		} else {
			imageMap[registryKey] = tempTags

		}
		mx.Unlock()

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

func (exporter Exporter) deleteRegistryImages(imageWithTag string, ctx context.Context) {

	ref, err := docker.ParseReference("//" + imageWithTag)
	if err != nil {

		exporter.Logger.Err(err).Msgf("error parsing docker image %s", imageWithTag)

	}

	img, err := ref.NewImage(ctx, nil)

	if err != nil {
		exporter.Logger.Err(err).Msgf("error referring the image %s", imageWithTag)

	}

	defer img.Close()
	fmt.Println("deleting image", img.Reference().DeleteImage(ctx, &types.SystemContext{}))

}
