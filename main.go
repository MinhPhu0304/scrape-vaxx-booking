package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
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
	resp, err := http.Get("https://raw.githubusercontent.com/CovidEngine/vaxxnzlocations/main/uniqLocations.json")
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

	for _, location := range locations {
		availability, err := getAvailability(location)
		if err != nil {
			log.Errorln(err)
			continue
		}
		vaxSlot, err := getAvailabilitySlots(availability, location)
		if err != nil {
			log.Errorln(err)
		}
		file, _ := json.MarshalIndent(combineAllSlots(vaxSlot), "", " ")
		err = ioutil.WriteFile("./slots/"+location.ExtId+"-test"+".json", file, 0644)
		if err != nil {
			log.Errorln(err)
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
		return LocationAvailability{}, errors.WithStack(err)
	}
	req.Header.Add("Accept", "application/JSON")
	req.Header.Add("User-Agent", "node-fetch/1.0 (+https://github.com/bitinn/node-fetch)")
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)

	if err != nil {
		return LocationAvailability{}, errors.WithStack(err)
	}
	defer resp.Body.Close()
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return LocationAvailability{}, errors.WithStack(err)
	}
	var availability LocationAvailability
	err = json.Unmarshal(bodyBytes, &availability)
	if err != nil {
		return LocationAvailability{}, errors.WithStack(err)
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

type SlotRequestBody struct {
	VaccineData string `json:"vaccineData"`
	GroupSize   int    `json:"groupSize"`
	Url         string `json:"url"`
	TimeZone    string `json:"timeZone"`
}

type SlotWithAvailability struct {
	LocalStartTime  string `json:"localStartTime"`
	DurationSeconds int    `json:"durationSeconds"`
}

type VaccineSlots struct {
	LocationExtId string                 `json:"locationExtId"`
	Date          string                 `json:"date"`
	Slots         []SlotWithAvailability `json:"slotsWithAvailability"`
}

func getAvailabilitySlots(locA LocationAvailability, location VaccineLocation) ([]VaccineSlots, error) {
	locationSlot := make([]VaccineSlots, 0)
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	for _, availDate := range locA.Availability {
		wg.Add(1)
		go func(l Availability) {
			select {
			case <-ctx.Done():
				return // Error somewhere, terminate
			default: // Default is must to avoid blocking
			}
			postBody, _ := json.Marshal(SlotRequestBody{
				VaccineData: "WyJhMVQ0YTAwMDAwMEdiVGdFQUsiXQ==",
				GroupSize:   1,
				Url:         "https://app.bookmyvaccine.covid19.health.nz/appointment-select",
				TimeZone:    "Pacific/Auckland",
			})
			postBodyBuffer := bytes.NewBuffer(postBody)

			client := &http.Client{}
			req, err := http.NewRequest("POST", "https://skl-api.bookmyvaccine.covid19.health.nz/public/locations/"+location.ExtId+"/date/"+l.Date+"/slots", postBodyBuffer)
			if err != nil {
				log.Errorln(errors.WithStack(err))
				return
			}
			req.Header.Add("Accept", "application/JSON")
			req.Header.Add("User-Agent", "node-fetch/1.0 (+https://github.com/bitinn/node-fetch)") // LOLLLL
			req.Header.Add("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				log.Errorln(errors.WithStack(err))
				return
			}
			if resp.StatusCode != http.StatusOK {
				cancel()
				log.Errorln(resp)
				return
			}
			defer resp.Body.Close()
			bodyBytes, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Errorln(errors.WithStack(err))
				return
			}
			var slots VaccineSlots
			err = json.Unmarshal(bodyBytes, &slots)
			if err != nil {
				log.Errorln(errors.WithStack(err))
				return
			}
			locationSlot = append(locationSlot, slots)
			defer wg.Done()
		}(availDate)
	}
	wg.Wait()
	defer cancel()
	return locationSlot, nil
}

type VaxAvailability map[string][]SlotWithAvailability

func combineAllSlots(slots []VaccineSlots) VaxAvailability {
	a := make(VaxAvailability)
	for _, slot := range slots {
		a[slot.Date] = slot.Slots
	}
	return a
}
