// Package fx provides EUR foreign-exchange reference rates from the ECB's
// historical file (eurofxref-hist.csv).
//
// Rates are quoted as 1 EUR = x CCY, so EUR = amount / rate.
package fx

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ichenhe/cgt-ie/internal/model"
)

// ECBHistURL is the ECB's full-history download (zip containing one CSV).
const ECBHistURL = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-hist.zip"

// MaxFallbackDays is how far back to look for a rate when the requested date
// has none (weekends, TARGET holidays).
const MaxFallbackDays = 7

// Rates is an in-memory table of daily EUR reference rates.
type Rates struct {
	byCCY map[string]map[time.Time]decimal.Decimal // ccy → date → rate
	dates []time.Time                              // all dates ascending
}

// Lookup is the result of a rate query.
type Lookup struct {
	Rate     decimal.Decimal
	Date     time.Time // date the rate is actually from
	Fallback bool      // true if Date != requested date
}

// Load reads an ECB eurofxref-hist.csv file.
func Load(path string) (*Rates, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

// Parse reads ECB CSV content: header "Date,USD,JPY,...", one row per
// TARGET business day, "N/A" for missing values.
func Parse(r io.Reader) (*Rates, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	if len(header) < 2 || strings.TrimPrefix(header[0], "\ufeff") != "Date" {
		return nil, fmt.Errorf("not an ECB eurofxref file (header %v)", header[:min(3, len(header))])
	}
	rates := &Rates{byCCY: map[string]map[time.Time]decimal.Decimal{}}
	seen := map[time.Time]bool{}
	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(row) == 0 || row[0] == "" {
			continue
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(row[0]))
		if err != nil {
			return nil, fmt.Errorf("bad date %q: %w", row[0], err)
		}
		for i := 1; i < len(row) && i < len(header); i++ {
			ccy := strings.TrimSpace(header[i])
			v := strings.TrimSpace(row[i])
			if ccy == "" || v == "" || v == "N/A" {
				continue
			}
			rate, err := decimal.NewFromString(v)
			if err != nil {
				return nil, fmt.Errorf("%s %s: bad rate %q", row[0], ccy, v)
			}
			m, ok := rates.byCCY[ccy]
			if !ok {
				m = map[time.Time]decimal.Decimal{}
				rates.byCCY[ccy] = m
			}
			m[d] = rate
		}
		if !seen[d] {
			seen[d] = true
			rates.dates = append(rates.dates, d)
		}
	}
	sort.Slice(rates.dates, func(i, j int) bool { return rates.dates[i].Before(rates.dates[j]) })
	if len(rates.dates) == 0 {
		return nil, fmt.Errorf("no rates found")
	}
	return rates, nil
}

// Latest returns the most recent date in the table.
func (r *Rates) Latest() time.Time { return r.dates[len(r.dates)-1] }

// Rate returns the EUR→ccy rate on date, falling back to the nearest earlier
// business day within MaxFallbackDays.
func (r *Rates) Rate(ccy string, date time.Time) (Lookup, error) {
	ccy = strings.ToUpper(ccy)
	if ccy == "EUR" {
		return Lookup{Rate: decimal.NewFromInt(1), Date: model.Day(date)}, nil
	}
	m, ok := r.byCCY[ccy]
	if !ok {
		return Lookup{}, fmt.Errorf("no ECB rates for currency %s", ccy)
	}
	want := model.Day(date)
	for back := 0; back <= MaxFallbackDays; back++ {
		d := want.AddDate(0, 0, -back)
		if rate, ok := m[d]; ok {
			return Lookup{Rate: rate, Date: d, Fallback: back > 0}, nil
		}
	}
	return Lookup{}, fmt.Errorf("no %s rate on or within %d days before %s",
		ccy, MaxFallbackDays, want.Format("2006-01-02"))
}

// ToEUR converts amount in ccy on date.
func (r *Rates) ToEUR(amount decimal.Decimal, ccy string, date time.Time) (decimal.Decimal, Lookup, error) {
	lk, err := r.Rate(ccy, date)
	if err != nil {
		return decimal.Zero, lk, err
	}
	return amount.DivRound(lk.Rate, 6), lk, nil
}

// Download fetches the ECB history zip and writes the CSV inside it to path.
func Download(path string) error {
	resp, err := http.Get(ECBHistURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", ECBHistURL, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return fmt.Errorf("unzip: %w", err)
	}
	for _, zf := range zr.File {
		if !strings.HasSuffix(strings.ToLower(zf.Name), ".csv") {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if _, err := Parse(bytes.NewReader(data)); err != nil {
			return fmt.Errorf("downloaded file does not parse: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, data, 0o644)
	}
	return fmt.Errorf("zip from %s contained no CSV", ECBHistURL)
}
