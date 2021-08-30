package main

import (
	"context"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"text/template"

	triton "github.com/joyent/triton-go"
	"github.com/joyent/triton-go/authentication"
	"github.com/joyent/triton-go/compute"
	"github.com/rs/zerolog"
)

var logger zerolog.Logger

func init() {

	logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()

}

type InventoryHost struct {
	HostName   string
	IP         string
	Tags       string
	Datacenter string
	UUID       string
	Image      string
	Package    string
	State      string
	Brand      string
}

var InventoryHosts []InventoryHost

type SDCOnfigs struct {
	Inventory []InventoryHost
}

func main() {

	ctx := context.Background()

	inventoryDcs := strings.Split(strings.ToLower(os.Getenv("TRITON_INVENTORY_DCS")), ",")
	for _, dcs := range inventoryDcs {
		
		cs, err := NewTritonClient(dcs)
		if err != nil {
			logger.Debug().Msg("error occured")

		}
		generate(cs, c, ctx)

	}

	var sdConfigs SDCOnfigs
	sdConfigs.Inventory = InventoryHosts

	templateFile := "ansible.tmpl"
	targetFilePath := "ansible.inventory"
	baseFileName := "ansible.tmpl"
	GenerateConfigFileFromTemplate(templateFile, targetFilePath, baseFileName, sdConfigs)

}

func generate(c *compute.ComputeClient, dc string, ctx context.Context) {

	mapTags := make(map[string]interface{})
	if os.Getenv("TRITON_IVENTORY_TAGS") != "" {
		splitTags := strings.Split(os.Getenv("TRITON_IVENTORY_TAGS"), "=")

		mapTags[splitTags[0]] = splitTags[1]
	}

	ci := c.Instances()

	li := &compute.ListInstancesInput{Tags: mapTags}
	instances, err := ci.List(ctx, li)
	if err != nil {
		logger.Err(err).Msg("error listing")
	}

	for _, instance := range instances {

		var i InventoryHost
		i.HostName = instance.Name
		i.IP = instance.PrimaryIP
		i.Datacenter = dc
		var tagString string
		for key, value := range instance.Tags {
			strKey := fmt.Sprintf("%v", key)
			strValue := fmt.Sprintf("%v", value)
			tagString = tagString + strKey + "=" + strValue + " "

		}
		i.Tags = tagString
		i.UUID = instance.ID

		i.Package = instance.Package
		i.State = instance.State
		i.Brand = instance.Brand
		i.Image = instance.Image

		InventoryHosts = append(InventoryHosts, i)
	}
}

func NewTritonClient(SDC_URL string) (*compute.ComputeClient, error) {

	keyMaterial := os.Getenv("TRITON_KEY_MATERIAL")
	keyID := os.Getenv("TRITON_SSH_KEY_ID")
	accountName := os.Getenv("TRITON_ACCOUNT")
	userName := ""
	var signer authentication.Signer
	var err error

	var keyBytes []byte
	if _, err = os.Stat(keyMaterial); err == nil {
		keyBytes, err = ioutil.ReadFile(keyMaterial)
		if err != nil {
			logger.Fatal().Err(err).Msg("Error reading key material")

		}
		block, _ := pem.Decode(keyBytes)
		if block == nil {
			logger.Fatal().Err(err).Msg("No key found")
		}

		if block.Headers["Proc-Type"] == "4,ENCRYPTED" {
			logger.Fatal().Err(err).Msg("password protected keys are\n" +
				"not currently supported. Please decrypt the key prior to use")
		}

	} else {
		keyBytes = []byte(keyMaterial)
	}

	input := authentication.PrivateKeySignerInput{
		KeyID:              keyID,
		PrivateKeyMaterial: keyBytes,
		AccountName:        accountName,
		Username:           userName,
	}

	signer, err = authentication.NewPrivateKeySigner(input)
	if err != nil {
		logger.Fatal().Err(err).Msg("Error Creating SSH Private Key Signer")

	}

	config := &triton.ClientConfig{
		TritonURL:   SDC_URL,
		AccountName: accountName,
		Username:    userName,
		Signers:     []authentication.Signer{signer},
	}

	c, err := compute.NewClient(config)
	if err != nil {
		logger.Fatal().Err(err).Msg("Compute new client")

	}
	return c, err
}

func GenerateConfigFileFromTemplate(templateFile string, targetFilePath string, baseFileName string, sdConfigs interface{}) error {

	t := template.Must(template.New(baseFileName).ParseFiles(templateFile))
	f, ferr := os.Create(targetFilePath)

	if ferr != nil {

		logger.Info().Err(ferr).Msgf("failed creating  target file")
		return ferr

	}

	defer f.Close()
	err := t.Execute(f, sdConfigs)
	if err != nil {
		logger.Info().Err(err).Msgf("failed apply template: ontinuing with out copying")
		return err
	}

	logger.Info().Msgf("completed generation of  config file for file")
	//locker.Unlock()
	return nil

}
