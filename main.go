package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
	"github.com/marpaia/graphite-golang"
	"github.com/mssola/user_agent"
)

// GlobalResult is exported to be parsed by json
type GlobalResult struct {
	Keywords  string         `json:"keywords"`
	URL       string         `json:"url"`
	UserAgent string         `json:"userAgent"`
	Device    string         `json:"mobile"`
	SEO       []googleResult `json:"naturals"`
	SEA       []googleResult `json:"annonceMethod2"`
	mutex     *sync.Mutex
}

func (gr GlobalResult) Print() {
	fmt.Println("results:")
	fmt.Printf("keywords: %s, url: %s, device: %s, user agent: %s\n", gr.Keywords, gr.URL, gr.Device, gr.UserAgent)
	fmt.Println("sea:")
	for _, sea := range gr.SEA {
		fmt.Printf("%d - %s - %s\n", sea.Position, sea.Domain, sea.Raw)
	}
	fmt.Println("seo:")
	for _, seo := range gr.SEO {
		fmt.Printf("%d - %s - %s\n", seo.Position, seo.Domain, seo.Raw)
	}
}

type googleResult struct {
	Position    int    `json:"position"`
	CSSSelector string `json:"cssSelector"`
	Raw         string `json:"raw"`
	Domain      string `json:"domain"`
}

func (gr *GlobalResult) exportToCSV() error {
	t := time.Now()
	file, err := os.Create("result.csv")
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()
	for _, value := range gr.SEA {
		domain := strings.Replace(value.Domain, ".", "_", -1)
		row := []string{t.Format("20060102150405"), "DT.hackhaton.2018.adwords." + gr.Device + ".sea." + domain, strconv.Itoa(value.Position)}
		fmt.Printf("write -> " + strings.Join(row, ";") + "\n")
		if err := writer.Write(row); err != nil {
			return err // let's return errors if necessary, rather than having a one-size-fits-all error handler
		}
	}
	return nil
}

func main() {
	// TODO constructor
	result := &GlobalResult{
		SEO:   make([]googleResult, 0),
		SEA:   make([]googleResult, 0),
		mutex: &sync.Mutex{},
	}

	// args
	keywords := os.Args[1]
	(*result).Keywords = keywords

	// build colly scrapper
	var userAgent string
	if os.Getenv("DEVICE") == "mobile" {
		userAgent = randMobile()
	} else {
		userAgent = randDesktop()
	}

	c := colly.NewCollector(
		colly.AllowedDomains("google.com", "www.google.com"),
		colly.UserAgent(userAgent),
	)

	// handler for retrieving SEA links
	c.OnHTML("body", func(body *colly.HTMLElement) {

		pos := -1
		body.ForEach("span", func(p int, span *colly.HTMLElement) {
			if span.Text == "Annonce" {
				pos = pos + 1
				found := false
				span.DOM.Siblings().EachWithBreak(func(p int, sibling *goquery.Selection) bool {
					domain := sibling.Text()
					if !strings.HasPrefix(domain, "http") {
						domain = "http://" + domain
					}
					URL, err := url.ParseRequestURI(domain)
					if err == nil {
						// found domain of the promoted link
						googleResult := googleResult{
							Position:    pos,
							CSSSelector: "span",
							Raw:         sibling.Text(),
							Domain:      URL.Hostname(),
						}
						result.SEA = append(result.SEA, googleResult)
						found = true
						return false
					}
					return true
				})
				if !found {
					googleResult := googleResult{
						Position:    pos,
						CSSSelector: "span",
						Raw:         "not found",
						Domain:      "unparseable",
					}
					result.SEA = append(result.SEA, googleResult)
				}
			}
		})
	})

	// handler for retrieving natural result
	c.OnHTML("div[id=ires]", func(div *colly.HTMLElement) {
		pos := -1
		span := "cite" // <span> or <cite> which contains found link by SEO
		if result.Device == "mobile" {
			span = "span"
		}
		div.ForEach(span, func(p int, span *colly.HTMLElement) {
			split := strings.Split(span.Text, " ")
			if len(split) < 1 {
				return
			}
			URL, err := url.ParseRequestURI(split[0])
			if err == nil {
				pos = pos + 1
				// found not promoted domain (seo)
				googleResult := googleResult{
					Position:    pos,
					CSSSelector: "div[id=ires]",
					Raw:         span.Text,
					Domain:      URL.Hostname(),
				}
				result.SEO = append(result.SEO, googleResult)
			} else {
				fmt.Errorf("can't parse span url %s\n:%v\n", span.Text, err)
			}
		})
	})

	// on request sent
	c.OnRequest(func(r *colly.Request) {
		fmt.Println("Request: ", r.URL.String())
		userAgent := r.Headers.Get("User-Agent")
		ua := user_agent.New(userAgent)
		if ua.Mobile() {
			if os.Getenv("DEVICE") != "mobile" {
				panic(errors.New("get a user agent mobile but script is not configured for a mobile (sytem environment variable DEVICE!='mobile'. user agent: " + userAgent))
			}
			result.Device = "mobile"
		} else {
			result.Device = "desktop"
			if os.Getenv("DEVICE") == "mobile" {
				panic(errors.New("get a user agent desktop but script is configured for a mobile (sytem environment variable DEVICE=='mobile'. user agent: " + userAgent))
			}
		}
		result.URL = r.URL.String()
		result.UserAgent = userAgent
	})

	// after the end of scrapping
	c.OnScraped(func(r *colly.Response) {
		result.Print()

		// looking for first occurence of oui.sncf in SEA and SEO parts
		firstOccurenceSEA := -1
		for _, sea := range result.SEA {
			if sea.Domain == "www.oui.sncf" {
				firstOccurenceSEA = sea.Position
				break
			}
		}
		firstOccurenceSEO := -1 // first occurence of oui.sncf in SEO part
		occurenceSEO := 0       // occurence of oui.sncf or sncf.com for density computing
		for _, seo := range result.SEO {
			if seo.Domain == "www.oui.sncf" {
				if firstOccurenceSEO == -1 {
					firstOccurenceSEO = seo.Position
				}
				occurenceSEO = occurenceSEO + 1
			}
			if seo.Domain == "www.sncf.com" {
				occurenceSEO = occurenceSEO + 1
			}
		}

		// compute waste (bidding is not necessary)
		waste := "0"
		if firstOccurenceSEA > -1 && firstOccurenceSEO > -1 {
			// oui.sncf is present in SEA and SEO
			ouiSpace := len(result.SEA) - firstOccurenceSEA - 1 + firstOccurenceSEO
			fmt.Printf("ouispace %d, len sea %d, sea %d, seo %d \n", ouiSpace, len(result.SEA), firstOccurenceSEA, firstOccurenceSEO)
			if ouiSpace == 0 {
				// there is no space between SEA position and SEO position for oui.sncf
				waste = "1"
			} else if ouiSpace == 1 && result.SEO[0].Domain == "www.sncf.com" {
				// there is just "www.sncf.com" between SEA and SEO oui.sncf
				waste = "1"
			}
		}

		// export to local file csv
		result.exportToCSV()

		// send to graphite
		fmt.Println("metrics sent to graphite (prefix: " + "DT.hackhaton.2018.adwords." + result.Device + "):")
		Graphite, _ := graphite.NewGraphiteWithMetricPrefix("10.98.208.116", 52630, "DT.hackhaton.2018.adwords."+result.Device)
		GraphiteNop, _ := graphite.GraphiteFactory("nop", "10.98.208.116", 52630, "DT.hackhaton.2018.adwords."+result.Device)

		if os.Getenv("MODE") == "prod" {
			Graphite.SimpleSend("sea.count", strconv.Itoa(len(result.SEA)))
			Graphite.SimpleSend("seo.count", strconv.Itoa(len(result.SEO)))
			Graphite.SimpleSend("waste", waste)
			Graphite.SimpleSend("seo.density", strconv.FormatFloat(float64(occurenceSEO)/float64(len(result.SEO)), 'f', 1, 64))
		}
		GraphiteNop.SimpleSend("sea.count", strconv.Itoa(len(result.SEA)))
		GraphiteNop.SimpleSend("seo.count", strconv.Itoa(len(result.SEO)))
		GraphiteNop.SimpleSend("seo.density", strconv.FormatFloat(float64(occurenceSEO)/float64(len(result.SEO)), 'f', 1, 64))
		GraphiteNop.SimpleSend("waste", waste)

		for _, sea := range result.SEA {
			domain := strings.Replace(sea.Domain, ".", "_", -1)

			if os.Getenv("MODE") == "prod" {
				Graphite.SimpleSend("sea."+domain, strconv.Itoa(sea.Position))
			}
			GraphiteNop.SimpleSend("sea."+domain, strconv.Itoa(sea.Position))
		}

		domains := make(map[string]int)
		for _, seo := range result.SEO {
			if _, ok := domains[seo.Domain]; ok {
				continue
			} else {
				domains[seo.Domain] = seo.Position
			}
			domain := strings.Replace(seo.Domain, ".", "_", -1)
			if os.Getenv("MODE") == "prod" {
				Graphite.SimpleSend("seo."+domain, strconv.Itoa(seo.Position))
			}
			GraphiteNop.SimpleSend("seo."+domain, strconv.Itoa(seo.Position))
		}
		fmt.Println("Finished", r.Request.URL)
	})

	// build the request
	var URL *url.URL
	URL, err := url.Parse("http://www.google.com")
	if err != nil {
		panic("boom")
	}
	URL.Path += "/search"
	parameters := url.Values{}
	parameters.Add("q", keywords)
	URL.RawQuery = parameters.Encode()
	fmt.Printf("url: %+v\n", URL.String())

	if err := c.Visit(URL.String()); err != nil {
		panic(err)
	}
}

var osMobileStrings = []string{
	"Mozilla/5.0 (iPhone; CPU iPhone OS 10_3_1 like Mac OS X) AppleWebKit/603.1.30 (KHTML, like Gecko) Version/10.0 Mobile/14E304 Safari/602.1",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 10_3_1 like Mac OS X) AppleWebKit/603.1.30 (KHTML, like Gecko) Version/10.0 Mobile/14E304 Safari/602.1",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 10_3_1 like Mac OS X) AppleWebKit/603.1.30 (KHTML, like Gecko) Version/10.0 Mobile/14E304 Safari/602.1",
	"Mozilla/5.0 (Linux; U; Android 4.4.2; en-us; SCH-I535 Build/KOT49H) AppleWebKit/534.30 (KHTML, like Gecko) Version/4.0 Mobile Safari/534.30",
	"Mozilla/5.0 (Linux; Android 7.0; SM-G930V Build/NRD90M) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/59.0.3071.125 Mobile Safari/537.36",
	"Mozilla/5.0 (Android 7.0; Mobile; rv:54.0) Gecko/54.0 Firefox/54.0",
	"Mozilla/5.0 (Android 7.0; Mobile; rv:54.0) Gecko/54.0 Firefox/54.0",
	"Mozilla/5.0 (Android 7.0; Mobile; rv:54.0) Gecko/54.0 Firefox/54.0",
	"Mozilla/5.0 (Linux; Android 7.0; SAMSUNG SM-G955U Build/NRD90M) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/5.4 Chrome/51.0.2704.106 Mobile Safari/537.36",
	"Mozilla/5.0 (Linux; Android 6.0; Nexus 5 Build/MRA58N) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.181 Mobile Safari/537.36",
}

var osDesktopStrings = []string{
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.181 Safari/537.3",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_10; rv:65.0.3325.146) Gecko/20100101 Firefox/65.0.3325.146",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_10) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/65.0.3325.146 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_10) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/65.0.3325.146 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; rv:65.0.3325.146) Gecko/20100101 Firefox/65.0.3325.146",
	"Mozilla/5.0 (Windows NT 5.1; rv:65.0.3325.146) Gecko/20100101 Firefox/65.0.3325.146",
	"Mozilla/5.0 (Windows NT 6.1; WOW64; rv:65.0.3325.146) Gecko/20100101 Firefox/65.0.3325.146",
	"Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/65.0.3325.146 Safari/537.36",
	"Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/65.0.3325.146 Safari/537.36",
	"Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/65.0.3325.146 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/65.0.3325.146 Safari/537.36",
}

func randDesktop() string {
	return osDesktopStrings[rand.Intn(len(osDesktopStrings))]
}

func randMobile() string {
	return osMobileStrings[rand.Intn(len(osDesktopStrings))]
}
