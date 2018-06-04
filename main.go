package main

import (
	"encoding/json"
	"fmt"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/extensions"
	"net/url"
	"os"
	"strings"
	"sync"
)

type GlobalResult struct {
	Keywords       string         `json:"keywords"`
	Url            string         `json:"url"`
	UserAgent      string         `json:"userAgent"`
	Naturals       []googleResult `json:"naturals"`
	AnnonceMethod1 []googleResult `json:"annonceMethod1"`
	AnnonceMethod2 []googleResult `json:"annonceMethod2"`
	mutex          *sync.Mutex
}

func (gr *GlobalResult) addNaturals(googleResult googleResult) {
	defer gr.mutex.Unlock()
	gr.mutex.Lock()
	gr.Naturals = append(gr.Naturals, googleResult)
}

type googleResult struct {
	Position    int    `json:"position"`
	CssSelector string `json:"cssSelector"`
	Raw         string `json:"raw"`
	Domain      string `json:"domain"`
}

func main() {
	// TODO constructor
	result := &GlobalResult{
		Naturals:       make([]googleResult, 0),
		AnnonceMethod1: make([]googleResult, 0),
		AnnonceMethod2: make([]googleResult, 0),
		mutex:          &sync.Mutex{},
	}
	// init
	pos_nat, pos_adwords := -1, -1
	defer func() {
		if pos_adwords == -1 {
			fmt.Println("pas de résultat acheté pour oui.sncf")
		}
		if pos_nat == -1 {
			fmt.Println("pas de résultat naturel trouvé pour oui.sncf")
		}
		if pos_nat <= pos_adwords && pos_nat > -1 {
			fmt.Println("référencement naturel est meilleur ou égal que le résultat adword")
		}
		if pos_nat > pos_adwords && pos_adwords > -1 {
			fmt.Println("résultat adword est meilleur que le référencement naturel")
		}
	}()

	// args
	keywords := os.Args[1]
	(*result).Keywords = keywords

	// build colly scrapper
	c := colly.NewCollector(
		colly.AllowedDomains("google.com", "www.google.com"),
	//	colly.UserAgent("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.139 Safari/537.36"),
	)
	extensions.RandomUserAgent(c)

	// handler for retrieving annonces from method1
	c.OnHTML("div[id=tvcap]", func(e *colly.HTMLElement) {
		fmt.Printf("Annonces found with method1: %q %s\n", e.Attr("id"), e.Attr("class"))
		e.ForEach("cite", func(pos int, elt *colly.HTMLElement) {
			fmt.Printf("%d - %+v\n", pos, elt.Text)
			domain := "unparseable"
			if !strings.HasPrefix(elt.Text, "http") {
				elt.Text = "http://" + elt.Text
			}
			Url, err := url.Parse(elt.Text)
			if err == nil {
				domain = Url.Hostname()
			}
			googleResult := googleResult{
				Position:    pos,
				CssSelector: "div[id=tvcap]",
				Raw:         elt.Text,
				Domain:      domain,
			}
			result.AnnonceMethod1 = append(result.AnnonceMethod1, googleResult)
			if strings.Contains(elt.Text, "oui.sncf") {
				pos_adwords = pos
			}
		})
	})

	// handler for retrieving annonces from method2
	c.OnHTML("span", func(e *colly.HTMLElement) {
		if e.Text == "Annonce" {
			annonceElt := e.DOM.Parent().Find("cite").First()
			fmt.Printf("==> annonce found: %+v\n", annonceElt.Text())
			domain := "unparseable"
			if !strings.HasPrefix(annonceElt.Text(), "http") {
				annonceElt.SetText("http://" + annonceElt.Text())
			}
			Url, err := url.ParseRequestURI(annonceElt.Text())
			if err == nil {
				domain = Url.Hostname()
			}
			googleResult := googleResult{
				Position:    -1,
				CssSelector: "span",
				Raw:         annonceElt.Text(),
				Domain:      domain,
			}
			result.AnnonceMethod2 = append(result.AnnonceMethod2, googleResult)

		}
	})

	// handler for retrieving natural result
	c.OnHTML("div[id=ires]", func(e *colly.HTMLElement) {
		fmt.Printf("Natural found: %q %s\n", e.Attr("id"), e.Attr("class"))
		e.ForEach("div[class=g]", func(pos int, elt *colly.HTMLElement) {
			if link, exists := elt.DOM.Find("a").First().Attr("href"); exists {
				fmt.Printf("%d - %+v\n", pos, link)
				domain := "unparseable"
				Url, err := url.Parse(link)
				if err == nil {
					domain = Url.Hostname()
				}
				googleResult := googleResult{
					Position:    pos,
					CssSelector: "div[id=ires]",
					Raw:         link,
					Domain:      domain,
				}
				result.addNaturals(googleResult)
				if strings.Contains(link, "oui.sncf") {
					pos_nat = pos
				}
			}
		})
	})

	// on request sent
	c.OnRequest(func(r *colly.Request) {
		fmt.Println("Request: ", r.URL.String())
		fmt.Println("User agent: ", r.Headers.Get("User-Agent"))
		result.Url = r.URL.String()
		result.UserAgent = r.Headers.Get("User-Agent")
	})

	//
	c.OnScraped(func(r *colly.Response) {
		prettyResult, err := json.MarshalIndent(result, "", "  ")
		if err == nil {
			fmt.Printf("result:\n%+v\n", string(prettyResult))
		}
		fmt.Println("Finished", r.Request.URL)
	})

	// build the request
	var Url *url.URL
	Url, err := url.Parse("http://www.google.com")
	if err != nil {
		panic("boom")
	}
	Url.Path += "/search"
	parameters := url.Values{}
	parameters.Add("q", keywords)
	Url.RawQuery = parameters.Encode()
	fmt.Printf("url: %+v\n", Url.String())

	if err := c.Visit(Url.String()); err != nil {
		panic(err)
	}
}
