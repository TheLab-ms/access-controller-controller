package client

import (
	"errors"
	"io"
	"strconv"
	"time"

	"golang.org/x/net/html"
)

func parseSwipesListPage(r io.Reader) ([]*CardSwipe, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	var cardSwipes []*CardSwipe
	var foundTable bool
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			foundTable = true
		}
		if n.Type == html.ElementNode && n.Data == "tr" {
			cardSwipe := &CardSwipe{}

			col := 0
			for td := n.FirstChild; td != nil; td = td.NextSibling {
				if td.Type != html.ElementNode || td.Data != "td" {
					continue
				}

				switch col {
				case 0:
					cardSwipe.ID, _ = strconv.Atoi(td.FirstChild.Data)
				case 1:
					cardSwipe.CardID, _ = strconv.Atoi(td.FirstChild.Data)
				case 2:
					cardSwipe.Name = td.FirstChild.Data
				case 3:
					cardSwipe.Status = td.FirstChild.Data
				case 4:
					cardSwipe.Time, err = time.Parse("2006-01-02 15:04:05", td.FirstChild.Data)
				}
				col++
			}

			if cardSwipe.ID != 0 {
				cardSwipes = append(cardSwipes, cardSwipe)
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}

	traverse(doc)
	if !foundTable {
		return nil, errors.New("no table found in access controller response")
	}
	return cardSwipes, nil
}
