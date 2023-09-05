package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

type bookingSession struct {
	SocialSecurityNumber         string  `json:"socialSecurityNumber"`         // "YYYYMMDD-XXXX"
	LicenceId                    int     `json:"licenceId"`                    // 4
	BookingModeId                int     `json:"bookingModeId"`                // 0
	IgnoreDebt                   bool    `json:"ignoreDebt"`                   // false
	IgnoreBookingHindrance       bool    `json:"ignoreBookingHindrance"`       // false
	ExaminationTypeId            int     `json:"examinationTypeId"`            // 0
	ExcludeExaminationCategories []any   `json:"excludeExaminationCategories"` // []
	RescheduleTypeId             int     `json:"rescheduleTypeId"`             // 0
	PaymentIsActive              bool    `json:"paymentIsActive"`              // false
	PaymentReference             *string `json:"paymentReference"`             // null,
	PaymentUrl                   *string `json:"paymentUrl"`                   //null,
	SearchedMonths               int     `json:"searchedMonths"`               // 0
}
type occasionBundleQuery struct {
	StartDate         string `json:"startDate"`         // "1970-01-01T00:00:00.000Z"
	SearchedMonths    int    `json:"searchedMonths"`    // 0
	LocationId        int    `json:"locationId"`        // 1000333,
	NearbyLocationIds []int  `json:"nearbyLocationIds"` // [1000302,1000334]
	LanguageId        int    `json:"languageId"`        // 13
	VehicleTypeId     int    `json:"vehicleTypeId"`     // 1
	TachographTypeId  int    `json:"tachographTypeId"`  // 1,
	OccasionChoiceId  int    `json:"occasionChoiceId"`  // 1
	ExaminationTypeId int    `json:"examinationTypeId"` // 10
}

type req struct {
	BookingSession      bookingSession      `json:"bookingSession"`
	OccasionBundleQuery occasionBundleQuery `json:"occasionBundleQuery"`
}

func newRequest(ssn string, location int, nearby []int) *req {
	return &req{
		BookingSession: bookingSession{
			SocialSecurityNumber:   ssn,
			LicenceId:              4,
			IgnoreBookingHindrance: true, // If set, can search even after license is granted
		},
		OccasionBundleQuery: occasionBundleQuery{
			StartDate:         "1970-01-01T00:00:00.000Z", // "1970-01-01T00:00:00.000Z",
			LocationId:        location,
			NearbyLocationIds: nearby,
			LanguageId:        13,
			VehicleTypeId:     1,
			TachographTypeId:  1,
			OccasionChoiceId:  1,
			ExaminationTypeId: 10,
		},
	}
}

//(cat /tmp/mctimes | jq ".data .bundles [0,1,2,3,4,5] .occasions [0] | .date,.time,.locationName"| grep -v null)

type occasion struct {
	Date         string `json:"date"`
	Time         string `json:"time"`
	LocationName string `json:"locationName"`
}
type bundle struct {
	Occasions []occasion `json:occasions`
}
type res struct {
	Bundles []bundle `json:"bundles"`
}

func doCheck(ssn string, location int, nearby []int) (string, error) {
	r := newRequest(ssn, location, nearby)
	jsonData, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	//fmt.Printf("-->\n%v\n", string(jsonData))

	req, err := http.NewRequest("POST", "https://fp.trafikverket.se/boka/occasion-bundles", bytes.NewReader(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/116.0")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	fmt.Printf("HTTP: %v\n ", resp.Status)
	response, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	//fmt.Printf("<--\n%v\n", string(response))
	var queryResponse res
	if err := json.Unmarshal(response, &queryResponse); err != nil {
		return "", err
	}
	if len(queryResponse.Bundles) > 0 {
		msg := []string{}
		for _, occ := range queryResponse.Bundles[0].Occasions {
			msg = append(msg, fmt.Sprintf("%s %s at %s", occ.Date, occ.Time, occ.LocationName))
		}
		return strings.Join(msg, "\n"), nil
	}
	return "", nil
}

func readSsns() ([]string, error) {
	pns, err := os.ReadFile("personnummer.txt")
	if err != nil {
		return nil, fmt.Errorf("Failed to read personnumer: %v", err)
	}
	// Split + remove empties
	var res []string
	for _, n := range strings.Split(strings.ReplaceAll(string(pns), "\r\n", "\n"), "\n") {
		if len(n) > 0 {
			res = append(res, n)
		}
	}
	return res, nil
}

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `

Examcheck is a utility to check available times for a driving exam, in Sweden. 
It currently only checks motorcycle exam times. 
In order to use it, you need to have a file, "personnummer.txt", containing personnummer, 
one per line. 

When a person completes their exam, the personnummer must be removed from the list, 
as it cannot be used for searches any more.

Regarding locations: they are defined numerically. By default, it searches
on Västra Haninge, Västra Haninge 2 and Gillinge. If you want to change this, 
you need to dig up the codes for the places you want to search for. 

Regarding notifications: install the app ntfy.sh and subscribe to the 'topic' that
you tell this program to use. That's it.

This program is supplied in the hope that it will be useful, that it will 
_decrease_ the load on trafikverket's pages and provide people with equal chances
of obtaining a slot. 

This program comes with no guarantees, use at your own discretion. 

`)
	fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	topic := flag.String("topic", "trafikcheck_129391", "topic to use for ntfy.sh")
	location1 := flag.Int("location1", 1000333, "primary location")
	location2 := flag.Int("location2", 1000302, "secondary location")
	location3 := flag.Int("location3", 1000334, "tertiary location")
	location4 := flag.Int("location4", 0, "quaternary location")
	flag.Usage = usage
	flag.Parse()

	var nearby []int
	if *location2 != 0 {
		nearby = append(nearby, *location2)
	}
	if *location3 != 0 {
		nearby = append(nearby, *location3)
	}
	if *location4 != 0 {
		nearby = append(nearby, *location4)
	}
	if err := loop(*topic, *location1, nearby); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func loop(topic string, location int, nearby []int) error {
	var lastlog time.Time
	for i := 0; ; i++ {
		// re-read config file
		ssns, err := readSsns()
		if err != nil {
			return err
		}
		if len(ssns) == 0 {
			return errors.New("No personnnumer present. Please add at least one.")
		}
		// pick a random ssn and search
		msg, err := doCheck(ssns[rand.Intn(len(ssns))], location, nearby)
		if err != nil {
			return err
		}
		if len(msg) > 0 {
			// Send notification via ntfy.sh
			http.Post(fmt.Sprintf("https://ntfy.sh/%s", topic), "text/plain",
				strings.NewReader(msg))
			// Additional sleep here
			time.Sleep(10 * time.Minute)
		}
		if time.Since(lastlog) > time.Hour {
			lastlog = time.Now()
			fmt.Printf("Checked %d times, using %d personnummer\n", i+1, len(ssns))
		}
		time.Sleep(30 * time.Second)
	}
	return nil
}
