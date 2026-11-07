package classify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

var _ Querier = &FIRDS{}

// FIRDS is ESMA's Financial Instruments Reference Data System: every
// instrument admitted to trading on an EU venue, with its ISO 10962 CFI code
// as reported by the issuer. Free, no key. Instruments traded only outside
// the EU (most UK-only listings since Brexit) are absent.
type FIRDS struct {
	HTTP *http.Client
	URL  string
	// BatchSize is how many ISINs go into one request (one OR clause each).
	BatchSize int
}

func NewFIRDS(client *http.Client) *FIRDS {
	if client == nil {
		client = http.DefaultClient
	}
	return &FIRDS{HTTP: client, URL: "https://registers.esma.europa.eu/solr/esma_registers_firds/select", BatchSize: 50}
}

func (*FIRDS) Name() string { return "FIRDS" }

func (f *FIRDS) Query(ctx context.Context, isins []string) (map[string]Info, error) {
	out := map[string]Info{}
	for i := 0; i < len(isins); i += f.BatchSize {
		part := isins[i:min(i+f.BatchSize, len(isins))]
		queryTerms := make([]string, len(part))
		for j, isin := range part {
			queryTerms[j] = `isin:"` + isin + `"`
		}
		q := url.Values{}
		q.Set("q", "("+strings.Join(queryTerms, " OR ")+") AND type_s:parent") // parent docs carry the CFI
		q.Set("wt", "json")
		q.Set("group", "true")
		q.Set("group.field", "isin")
		q.Set("group.limit", "1")
		q.Set("rows", fmt.Sprint(len(part)))
		q.Set("fl", "isin,gnr_cfi_code")

		var resp struct {
			Grouped struct {
				ISIN struct {
					Groups []struct {
						DocList struct {
							Docs []struct {
								ISIN string `json:"isin"`
								CFI  string `json:"gnr_cfi_code"`
							} `json:"docs"`
						} `json:"doclist"`
					} `json:"groups"`
				} `json:"isin"`
			} `json:"grouped"`
		}
		if err := getJSON(ctx, f.HTTP, f.URL+"?"+q.Encode(), &resp); err != nil {
			return out, err
		}
		for _, g := range resp.Grouped.ISIN.Groups {
			for _, d := range g.DocList.Docs {
				if d.CFI != "" {
					out[strings.ToUpper(d.ISIN)] = Info{CFI: d.CFI}
				}
			}
		}
	}
	return out, nil
}
