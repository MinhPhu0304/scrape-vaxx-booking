package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

type Location struct {
	Lat  float64 `json:"lat"`
	Long float64 `json:"lng"`
}

type VaccineLocation struct {
	VaccineData      string   `json:"vaccineData"`
	Type             string   `json:"type"`
	Location         Location `json:"location"`
	ExtId            string   `json:"extId"`
	RegionExternalId string   `json:"regionExternalId"`
	DisplayAddress   string   `json:"displayAddress"`
}

func main() {
	log.Println("Start scraping")
	resp, err := http.Get("https://raw.githubusercontent.com/CovidEngine/vaxxnzlocations/main/uniqLocationsNew.json")
	if err != nil {
		log.Fatalln(err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}

	var locations []VaccineLocation
	err = json.Unmarshal(body, &locations)
	if err != nil {
		log.Fatal(err)
	}

	for index, location := range locations {
		if index < 1 {
			_, err := getAvailability(location)
			if err != nil {
				log.Println(err)
			}
		}
	}
}

type AvailabilityRequest struct {
	EndDate     string `json:"endDate"`
	StartDate   string `json:"startDate"`
	VaccineData string `json:"vaccineData"`
	GroupSize   int    `json:"groupSize"`
	DoseNumber  int    `json:"doseNumber"`
	Url         string `json:"url"`
	TimeZone    string `json:"timeZone"`
}

type Availability struct {
	Date        string `json:"date"`
	Available   bool   `json:"available"`
	VaccineData string `json:"vaccineData"`
}

type LocationAvailability struct {
	LocationExtId string         `json:"locationExtId"`
	VaccineData   string         `json:"vaccineData"`
	Availability  []Availability `json:"availability"`
}

func getAvailability(location VaccineLocation) (LocationAvailability, error) {
	now := time.Now()
	startDate := now.UTC()
	endDate := now.AddDate(0, 2, 0).UTC()
	postBody, _ := json.Marshal(AvailabilityRequest{
		EndDate:     endDate.Format("2006-01-02"),
		StartDate:   startDate.Format("2006-01-02"),
		VaccineData: "WyJhMVQ0YTAwMDAwMEdiVGdFQUsiXQ==",
		GroupSize:   1,
		DoseNumber:  1,
	})
	responseBody := bytes.NewBuffer(postBody)

	client := &http.Client{}
	req, err := http.NewRequest("POST", "https://skl-api.bookmyvaccine.covid19.health.nz/public/locations/"+location.ExtId+"/availability", responseBody)
	if err != nil {
		fmt.Print(err.Error())
		return LocationAvailability{}, err
	}
	req.Header.Add("Accept", "application/JSON")
	req.Header.Add("User-Agent", "node-fetch/1.0 (+https://github.com/bitinn/node-fetch)")
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)

	if err != nil {
		return LocationAvailability{}, err
	}
	defer resp.Body.Close()
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return LocationAvailability{}, err
	}
	var availability LocationAvailability
	err = json.Unmarshal(bodyBytes, &availability)
	if err != nil {
		return LocationAvailability{}, err
	}

	return LocationAvailability{
		LocationExtId: availability.LocationExtId,
		VaccineData:   availability.VaccineData,
		Availability:  filterOutUnavailable(availability.Availability),
	}, nil
}

func filterOutUnavailable(a []Availability) []Availability {
	availability := make([]Availability, 0)
	for _, time := range a {
		if time.Available {
			availability = append(availability, time)
		}
	}
	return availability
}
