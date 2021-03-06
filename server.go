package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/lambdaspace/LambdaSpaceAPIv2/mqtt"
)

type PeopleNowPresent struct {
	Value int `json:"value"`
}

type SpaceDescriptor struct {
	sync.RWMutex
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

type Status struct {
	Open             bool  `json:"open"`
	PeopleNowPresent int   `json:"people_now_present"`
	Lastchange       int64 `json:"lastchange"`
}

type HackerspaceEvents struct {
	Title string `json:"title"`
	Date  string `json:"date"`
	Begin string `json:"begin"`
	End   string `json:"end"`
	URL   string `json:"url"`
}

type DiscourseApi struct {
	TopicList struct {
		Topics []struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
			Slug  string `json:"slug"`
		} `json:"topics"`
	} `json:"topic_list"`
}

var (
	statusChan      = make(chan int)
	lastChange      *int64
	peoplePresent   *int
	spaceDescriptor SpaceDescriptor
	spaceStatus     *bool
	spaceEvents     struct {
		sync.RWMutex
		Events []HackerspaceEvents `json:"events"`
	}
)

func init() {
	spaceDescriptor = getJSONFile("./LambdaSpaceAPI.json")
	peoplePresent = &spaceDescriptor.Sensors.PeopleNowPresent[0].Value
	spaceStatus = &spaceDescriptor.State.Open
	lastChange = &spaceDescriptor.State.Lastchange
	go func() {
		go updateStatus()
		go getScheduledEvents()
		for range time.Tick(time.Minute * 5) {
			go getScheduledEvents()
		}
	}()
}

func check(err error) {
	if err != nil {
		log.Println(err)
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

// Make a http request and return it's body

func fetchHTTPResource(requestURL string) ([]byte, error) {
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}
	response, err := httpClient.Get(requestURL)
	check(err)
	defer response.Body.Close()
	responseBody, err := ioutil.ReadAll(response.Body)
	check(err)
	return responseBody, err
}

// Update the current status of the space
func updateStatus() {
	for numOfPeople := range statusChan {
		spaceDescriptor.Lock()
		previousPeoplePresent := *peoplePresent
		*peoplePresent = numOfPeople
		*spaceStatus = *peoplePresent > 0
		if previousPeoplePresent != *peoplePresent {
			*lastChange = time.Now().Unix()
		}
		spaceDescriptor.Unlock()
	}
}

// Get upcoming events
func getScheduledEvents() {
	dat := DiscourseApi{}
	ret := []HackerspaceEvents{}

	response, err := fetchHTTPResource("https://community.lambdaspace.gr/c/events.json")

	if err != nil {
		log.Print(err)
		return
	}
	err = json.Unmarshal(response, &dat)
	check(err)

	for _, element := range dat.TopicList.Topics {
		event := HackerspaceEvents{}
		id := element.ID
		slug := element.Slug
		fields := strings.Fields(element.Title)
		yearRegexp, err := regexp.Compile(`^\d\d\/\d\d\/\d\d\d\d`)
		check(err)
		hourRegexp, err := regexp.Compile(`^\d\d:\d\d`)
		check(err)

		if yearRegexp.MatchString(fields[0]) {
			event.Date = fields[0]
			event.URL = fmt.Sprintf("https://community.lambdaspace.gr/t/%s/%v", slug, id)
			if hourRegexp.MatchString(fields[1]) {
				event.Begin = fields[1]
				if fields[2] == "-" && hourRegexp.MatchString(fields[3]) {
					event.End = fields[3]
					event.Title = strings.Join(fields[4:], " ")
				} else {
					event.End = ""
					event.Title = strings.Join(fields[2:], " ")
				}
				ret = append(ret, event)
			}
		}
	}
	if len(ret) > 0 {
		spaceEvents.Lock()
		spaceEvents.Events = ret
		spaceEvents.Unlock()
	}
}

func spaceAPIRouteHandler(c echo.Context) error {
	spaceDescriptor.RLock()
	defer spaceDescriptor.RUnlock()
	return c.JSON(http.StatusOK, &spaceDescriptor)
}

func main() {
	e := echo.New()
	go mqtt.Main(statusChan)

	// Echo middlewares
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowMethods: []string{echo.GET},
	}))

	// Route compatible with SpaceApi spec
	e.GET("/api/v2.0/SpaceAPI", spaceAPIRouteHandler)

	// Route to serve space status
	e.GET("/api/v2.0/status", func(c echo.Context) error {
		spaceDescriptor.RLock()
		defer spaceDescriptor.RUnlock()
		return c.JSON(http.StatusOK, Status{*spaceStatus, *peoplePresent, *lastChange})
	})

	// Route to serve events
	e.GET("/api/v2.0/events", func(c echo.Context) error {
		spaceEvents.RLock()
		defer spaceEvents.RUnlock()
		return c.JSON(http.StatusOK, &spaceEvents)
	})

	// Legacy route to support migration from hackers.txt
	// Route to serve hackers
	e.GET("/api/v2.0/hackers", func(c echo.Context) error {
		spaceEvents.RLock()
		defer spaceEvents.RUnlock()
		return c.String(http.StatusOK, strconv.Itoa(*peoplePresent))
	})

	// Add custom httpServer so we can add timeouts to requests
	httpServer := &http.Server{
		Addr:         ":1323",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	e.Logger.Fatal(e.StartServer(httpServer))
}
