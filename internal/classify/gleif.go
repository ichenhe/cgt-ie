package classify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// GLEIF is the Global LEI Foundation's register. Given an ISIN it returns
// the issuer's LEI record, whose entity category says whether the issuer is
// a fund or an ordinary company. Coverage depends on issuers reporting their
// ISIN→LEI links, so many funds are missing.
type GLEIF struct {
	HTTP *http.Client
	URL  string
}

func NewGLEIF(client *http.Client) *GLEIF {
	if client == nil {
		client = http.DefaultClient
	}
	return &GLEIF{HTTP: client, URL: "https://api.gleif.org/api/v1/lei-records"}
}

func (*GLEIF) Name() string { return "GLEIF" }

// Query implements Querier. GLEIF has no batch endpoint; one request per ISIN.
func (g *GLEIF) Query(ctx context.Context, isins []string) (map[string]Info, error) {
	out := map[string]Info{}
	for _, isin := range isins {
		q := url.Values{}
		q.Set("filter[isin]", isin)
		var resp struct {
			Data []struct {
				Attributes struct {
					Entity struct {
						Category  string `json:"category"`
						LegalName struct {
							Name string `json:"name"`
						} `json:"legalName"`
					} `json:"entity"`
				} `json:"attributes"`
			} `json:"data"`
		}
		if err := getJSON(ctx, g.HTTP, g.URL+"?"+q.Encode(), &resp); err != nil {
			return out, err
		}
		if len(resp.Data) == 0 {
			continue
		}
		e := resp.Data[0].Attributes.Entity
		info := Info{IssuerName: e.LegalName.Name}
		// GLEIF categories: GENERAL, FUND, BRANCH, SOLE_PROPRIETOR,
		// RESIDENT_GOVERNMENT_ENTITY, INTERNATIONAL_ORGANIZATION.
		switch e.Category {
		case "FUND":
			info.Issuer = IssuerFund
		case "GENERAL":
			info.Issuer = IssuerCompany
		default:
			continue
		}
		out[strings.ToUpper(isin)] = info
	}
	return out, nil
}

// getJSON performs a GET and decodes a JSON body.
func getJSON(ctx context.Context, hc *http.Client, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("%s: HTTP %d: %s", req.URL.Host, res.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(res.Body).Decode(v)
}
