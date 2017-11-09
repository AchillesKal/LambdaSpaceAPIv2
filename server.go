package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo"
)

type PeopleNowPresent struct {
	Value int `json:"value"`
}

type SpaceDescriptor struct {
	API      string `json:"api"`
	Space    string `json:"space"`
	Logo     string `json:"logo"`
	URL      string `json:"url"`
	Location struct {
		Address string  `json:"address"`
		Lon     float64 `json:"lon"`
		Lat     float64 `json:"lat"`
	} `json:"location"`
	State struct {
		Open       bool  `json:"open"`
		Lastchange int64 `json:"lastchange"`
	} `json:"state"`
	Contact struct {
		Email      string `json:"email"`
		Irc        string `json:"irc"`
		Ml         string `json:"ml"`
		Twitter    string `json:"twitter"`
		Facebook   string `json:"facebook"`
		Foursquare string `json:"foursquare"`
	} `json:"contact"`
	Sensors struct {
		PeopleNowPresent []PeopleNowPresent `json:"people_now_present"`
	} `json:"sensors"`
	IssueReportChannels []string `json:"issue_report_channels"`
	Projects            []string `json:"projects"`
	Cache               struct {
		Schedule string `json:"schedule"`
	} `json:"cache"`
}

func check(err error) {
	if err != nil {
		fmt.Println(err)
		// panic(err)
	}
}

// Read file from file system
func readFile(fileName string) []byte {
	file, err := ioutil.ReadFile(fileName)
	check(err)
	return file
}

// Read json file
func getJSONFile(fileName string) (jsonFile SpaceDescriptor) {
	tmp := readFile(fileName)
	err := json.Unmarshal(tmp, &jsonFile)
	check(err)
	return
}

// Get number of people in space
func peopleInSpace() int {
	req, err := http.NewRequest("GET", "https://lambdaspace.gr/hackers.txt", strings.NewReader(""))
	check(err)
	resp, err := http.DefaultClient.Do(req)
	check(err)
	defer resp.Body.Close()
	b, _ := ioutil.ReadAll(resp.Body)
	data, _ := strconv.Atoi(string(b))
	return data
}

func main() {
	e := echo.New()
	spaceDescriptor := getJSONFile("./LambdaSpaceAPI.json")
	e.GET("/api/v2.0/SpaceAPI", func(c echo.Context) error {
		// To change the state of the space do:
		// spaceDescriptor.State.Open = true
		spaceDescriptor.Sensors.PeopleNowPresent[0].Value = peopleInSpace()
		spaceDescriptor.State.Open = spaceDescriptor.Sensors.PeopleNowPresent[0].Value > 0
		return c.JSON(http.StatusOK, spaceDescriptor)
	})

	// Route to serve space status
	e.GET("/api/v2.0/status", func(c echo.Context) error {
		spaceDescriptor.Sensors.PeopleNowPresent[0].Value = peopleInSpace()
		spaceDescriptor.State.Open = spaceDescriptor.Sensors.PeopleNowPresent[0].Value > 0
		return c.JSON(http.StatusOK, spaceDescriptor.State)
	})

	// Route to serve events
	// e.GET("/api/v2.0/events", func(c echo.Context) error {
	//  return
	// })
	e.Logger.Fatal(e.Start(":1323"))
}
