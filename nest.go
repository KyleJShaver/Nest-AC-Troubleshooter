package main

import (
	"os"
	"strings"
	"io/ioutil"
	"fmt"
	"encoding/json"
	"strconv"
	"net/http"
	"errors"
	"time"
	"net/url"
)

type NestConfig struct {
	Token        string `json:"token"`
	ThermostatID string `json:"thermostat_id"`
	Minutes      int `json:"minutes"`
	Output       string `json:"output"`
	LastOutput   string `json:"last_output"`
	Debug        bool `json:"debug"`
	WebhookPost  string `json:"webhook_post"`
	WebhookGet  string `json:"webhook_get"`
}

type NestCommand string

type NestData struct {
	CurrentTemperature int
	HvacMode           string
	IsCooling          bool
}

const (
	ThermostatID NestCommand = "thermostatID"
	Token NestCommand = "token"
	Minutes NestCommand = "minute"
	Config NestCommand = "config"
	Output NestCommand = "output"
	WebhookPost NestCommand = "webhook-post"
	WebhookGet NestCommand = "webhook-get"
)

const (
	NEST_GET string = "https://developer-api.nest.com"
	NEST_PUT string = "https://developer-api.nest.com/devices/thermostats/"
)

func main() {
	config := NestConfig{Minutes:10, Debug:false, Output:"nest.tsv", LastOutput:"nest_last.json"}
	allowedSingles := map[string]func() {"help": displayHelp, "-h": displayHelp, "--help": displayHelp}
	if len(os.Args) == 1 {
		expectedArguments()
		return
	}
	if len(os.Args) == 2 {
		singleCheck := strings.ToLower(os.Args[1])
		if allowedSingles[singleCheck] == nil {
			unexpectedArgument()
			return
		}
		allowedSingles[singleCheck]()
		return
	}
	argTranslate := map[string]NestCommand {
		"--thermostat": ThermostatID, "-id": ThermostatID,
		"--token": Token, "-t": Token,
		"--minute": Minutes, "-m": Minutes,
		"--config": Config, "-c": Config,
		"--output": Output, "-o": Output,
		"--webhook-post": WebhookPost, "-wp": WebhookPost,
		"--webhook-get": WebhookGet, "-wg": WebhookGet,
	}
	argMap := map[NestCommand]string {}
	for i:= 1; i < len(os.Args) - 1; i+=2 {
		argName := strings.ToLower(os.Args[i])
		if allowedSingles[argName] != nil || argTranslate[argName] == "" {
			unexpectedArgument()
			return
		}
		argMap[argTranslate[argName]] = os.Args[i + 1]
	}
	if configFile := argMap[Config]; configFile != "" {
		configOverride, err := processConfig(configFile, config)
		if err != nil {
			printError("Error reading config file", err)
			return
		}
		config = configOverride
	}
	if thermostatID := argMap[ThermostatID]; thermostatID != "" {
		config.ThermostatID = thermostatID
	} else if config.ThermostatID == "" {
		printError("Must provide a thermostat ID", nil)
		return
	}
	if token := argMap[Token]; token != "" {
		config.Token = token
	} else if config.Token == "" {
		printError("Must provide a token", nil)
		return
	}
	if minutesStr := argMap[Minutes]; minutesStr != "" {
		minutesInt, err := strconv.Atoi(minutesStr)
		if err != nil {
			printError("minutes parameter is not a parseable number", err)
			return
		}
		if minutesInt < 1 {
			printError("minutes parameter must be at least 1", nil)
			return
		}
		config.Minutes = minutesInt
	}
	if output := argMap[Output]; output != "" {
		config.Output = output
	}
	if webhookPost := argMap[WebhookPost]; webhookPost != "" {
		_, err := url.ParseRequestURI(webhookPost)
		if err != nil {
			printError("invalid webhook-post url", err)
			return
		}
		config.WebhookPost = webhookPost
	}
	if webhookGet := argMap[WebhookGet]; webhookGet != "" {
		_, err := url.ParseRequestURI(webhookGet)
		if err != nil {
			printError("invalid webhook-get url", err)
			return
		}
		config.WebhookGet = webhookGet
	}
	loop(config)
}

func expectedArguments() {
	printError("Expected at least one parameter. Please add arguments or call '--help' for help\n", nil)
}

func unexpectedArgument() {
	printError("Unrecognized or unexpected parameter. Please check your inputs or call '--help' for help\n", nil)
}

func displayHelp()  {
	fmt.Printf("--help, -h, help            display this help menu\n")
	fmt.Printf("--thermostat -id [id]       supply the thermostat id\n")
	fmt.Printf("--token -t [auth token]     supply the OAuth token\n")
	fmt.Printf("--minute -m [1-99]          set sampling time in minutes\n")
	fmt.Printf("--config -c [config file]   location of config file\n")
	fmt.Printf("--output -o [output file]   where to save the output .tsv file\n")
	fmt.Printf("--webhook-post -wp [url]    webhook post to be fired when system restart begins\n")
	fmt.Printf("--webhook-get -wg [url]     webhook get to be fired when system restart begins\n")
}

func printError(message string, err error) {
	if err == nil {
		fmt.Printf("%s\n", message)
		return
	}
	fmt.Printf("%s:\n%s\n", message, err.Error())
}

func printErrorExit(message string, err error) {
	printError(message, err)
	os.Exit(1)
}

func processConfig(fileName string, config NestConfig) (NestConfig, error) {
	var configOverride NestConfig
	fileContent, err := ioutil.ReadFile(fileName)
	if err != nil {
		return NestConfig{}, err
	}
	err = json.Unmarshal(fileContent, &configOverride)
	if err != nil {
		return NestConfig{}, err
	}
	if configOverride.Minutes < 1 {
		configOverride.Minutes = config.Minutes
	}
	if configOverride.Output == "" {
		configOverride.Output = config.Output
	}
	if configOverride.LastOutput == "" {
		configOverride.LastOutput = config.LastOutput
	}
	return configOverride, nil
}

func loop(config NestConfig) {
	outputFile, err := os.Create(config.Output)
	if err != nil {
		printError("Problem creating output file", err)
	}
	outputFile.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\n", "timestamp", "cooling", "temp", "notes"))
	outputFile.Sync()
	outputFile.Close()
	lastTemp := 0
	lastIsCooling := false
	for true {
		outputFile, err = os.OpenFile(config.Output, os.O_RDWR | os.O_APPEND, 0666)
		if err != nil {
			printError(fmt.Sprintf("Problem reopening output file %s", config.Output), err)
		}
		thermostatData, err := nestGet(config)
		if err != nil {
			outputFile.WriteString(fmt.Sprintf("%s\t\t\t%s\n", timeAsStr(), err.Error()))
			printError("Error in GET request", err)
		} else {
			outputFile.WriteString(fmt.Sprintf("%s\t%t\t%d\n",
				timeAsStr(),
				thermostatData.IsCooling,
				thermostatData.CurrentTemperature))
			if lastIsCooling == true && thermostatData.IsCooling == true {
				if lastTemp < thermostatData.CurrentTemperature {
					outputFile.WriteString(fmt.Sprintf("%s\t\t\t%s\n",
						timeAsStr(),
						"RESTARTING SYSTEM"))
					go func(){
						if config.WebhookPost != "" {
							err := webhook("POST", config.WebhookPost)
							if err != nil {
								printError("Problem with webhook-post", err)
							} else {
								outputFile.WriteString(fmt.Sprintf("%s\t\t\t%s\n",
									timeAsStr(),
									"webhook-post performed"))
							}
						}
						if config.WebhookGet != "" {
							err := webhook("GET", config.WebhookGet)
							if err != nil {
								printError("Problem with webhook-get", err)
							} else {
								outputFile.WriteString(fmt.Sprintf("%s\t\t\t%s\n",
									timeAsStr(),
									"webhook-get performed"))
							}
						}
					}()
					shutoffData := NestData{HvacMode: "unrestarted"}
					var err error
					for shutoffData.HvacMode != "off" {
						shutoffData, err = nestPut(config, "off")
						if err != nil {
							outputFile.WriteString(fmt.Sprintf("%s\t\t\t%s\n", timeAsStr(), errors.New(fmt.Sprintf("Error turning system off:\n%s", err.Error()))))
							printError("Error turning system off", err)
						} else if shutoffData.HvacMode != "off" {
							outputFile.WriteString(fmt.Sprintf("%s\t\t\t%s\n", timeAsStr(), "RESTART: waiting on turn off"))
							printError("RESTART: waiting on turn off", nil)
						} else {
							outputFile.WriteString(fmt.Sprintf("%s\t\t\t%s\n", timeAsStr(), "RESTART: system turned off"))
						}
					}
					restartData := NestData{HvacMode: "unrestarted"}
					for restartData.HvacMode != thermostatData.HvacMode {
						restartData, err = nestPut(config, thermostatData.HvacMode)
						if err != nil {
							outputFile.WriteString(fmt.Sprintf("%s\t\t\t%s\n", timeAsStr(), errors.New(fmt.Sprintf("Error turning system back on:\n%s", err.Error()))))
							printError("Error turning system back on", err)
						} else if restartData.HvacMode != thermostatData.HvacMode {
							outputFile.WriteString(fmt.Sprintf("%s\t\t\t%s\n", timeAsStr(), "RESTART: waiting on turn on"))
							printError("RESTART: waiting on turn on", nil)
						} else {
							outputFile.WriteString(fmt.Sprintf("%s\t\t\t%s\n", timeAsStr(), "RESTART: system turned on"))
						}
					}
					lastIsCooling = restartData.IsCooling
					lastTemp = restartData.CurrentTemperature
				} else {
					lastIsCooling = thermostatData.IsCooling
					lastTemp = thermostatData.CurrentTemperature
				}
			} else {
				lastIsCooling = thermostatData.IsCooling
				lastTemp = thermostatData.CurrentTemperature
			}
		}
		outputFile.Sync()
		outputFile.Close()
		time.Sleep(time.Minute * time.Duration(config.Minutes))
	}
}

func timeAsStr() string {
	return time.Now().Format(time.RFC850)

}

func nestClient(req *http.Request) http.Client {
	return http.Client {
		CheckRedirect: func(redirRequest *http.Request, via []*http.Request) error {
			redirRequest.Header = req.Header
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}
}

func nestGet(config NestConfig) (NestData, error) {
	req, err := http.NewRequest("GET", NEST_GET, nil)
	if err != nil {
		return NestData{}, errors.New(fmt.Sprintf("%s:\n%s\n", "Could not create GET request with included URL", err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", config.Token)
	client := nestClient(req)
	resp , err := client.Do(req)
	if err != nil {
		return NestData{}, errors.New(fmt.Sprintf("%s:\n%s\n", "Could not communicate with Nest", err))
	}
	if resp.StatusCode != 200 {
		return NestData{}, errors.New(fmt.Sprintf("%s:\n%s\n", "Problem getting an answer from Nest", err))
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return NestData{}, errors.New(fmt.Sprintf("%s:\n%s\n", "Could not read response from Nest", err))
	}
	if config.Debug == true {
		file, err := os.Create(config.LastOutput)
		if err != nil {
			return NestData{}, errors.New(fmt.Sprintf("%s:\n%s\n", "Could not create debug dump file", err))
		}
		file.Write(body)
		file.Sync()
		file.Close()
	}
	var rawData interface{}
	err = json.Unmarshal(body, &rawData)
	if err != nil {
		return NestData{}, errors.New(fmt.Sprintf("%s:\n%s\n", "Could not process JSON file from Nest", err))
	}
	dataNavigator := rawData.(map[string]interface{})
	dataPath := []string {"devices", "thermostats", config.ThermostatID}
	i := 0
	for dataNavigator != nil && i < len(dataPath) {
		dataNavigator = dataNavigator[dataPath[i]].(map[string]interface{})
		i += 1
	}
	if dataNavigator == nil || i < len(dataPath) {
		return NestData{}, errors.New(fmt.Sprintf("%s:\n%s\n", "Could not find thermoastat in Nest GET call", err))
	}
	thermostatData := NestData{}
	thermostatData.CurrentTemperature = int(dataNavigator["ambient_temperature_f"].(float64))
	thermostatData.HvacMode = dataNavigator["hvac_mode"].(string)
	thermostatData.IsCooling = dataNavigator["hvac_state"].(string) == "cooling"
	return thermostatData, nil
}

func nestPut(config NestConfig, hvacMode string) (NestData, error) {
	payload := strings.NewReader(fmt.Sprintf("{\"hvac_mode\": \"%s\"}", hvacMode))
	req, err := http.NewRequest("PUT", fmt.Sprintf("%s%s", NEST_PUT, config.ThermostatID), payload)
	if err != nil {
		return NestData{}, errors.New(fmt.Sprintf("%s:\n%s\n", "Could not create PUT request with included URL", err))
	}
	req.Header.Set("Authorization", config.Token)
	client := nestClient(req)
	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return NestData{}, errors.New(fmt.Sprintf("%s:\n%s\n", "Could not communicate with Nest", err))
	}
	if resp.StatusCode != 200 {
		return NestData{}, errors.New(fmt.Sprintf("%s:\n%s\n", "Problem getting an answer from Nest", err))
	}
	time.Sleep(time.Minute * 1)
	return nestGet(config)
}

func webhook(method, url string) error {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return errors.New(fmt.Sprintf("%s:\n%s\n", "Could not create request with webhook url", err))
	}
	client := nestClient(req)
	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil || resp.StatusCode != 200 {
		return errors.New(fmt.Sprintf("%s:\n%s\n", "Problem communicating with webhook", err))
	}
	return nil
}