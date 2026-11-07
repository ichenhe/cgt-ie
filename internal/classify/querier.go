package classify

import (
	"context"
	"fmt"
	"strings"
)

// IssuerKind is the normalised legal nature of an instrument's issuer.
type IssuerKind string

const (
	IssuerUnknown IssuerKind = ""
	IssuerFund    IssuerKind = "fund"    // a collective investment vehicle
	IssuerCompany IssuerKind = "company" // an operating company or SPV; not a fund
)

// Info is additional information of one instrument that may deduce its tax
// regime. Zero fields means "unknown".
type Info struct {
	// CFI is the ISO 10962 classification.
	CFI string `json:"cfi,omitempty"`
	// Issuer is the normalised kind of the issuing entity.
	Issuer IssuerKind `json:"issuer,omitempty"`
	// IssuerName is the issuer's legal name.
	IssuerName string `json:"issuer_name,omitempty"`
}

// known reports whether the Info carries anything to make a solid decision.
func (i Info) known() bool { return i.CFI != "" || i.Issuer != IssuerUnknown }

type Querier interface {
	Name() string
	// Query returns Info for the ISINs it knows. ISINs it does not know are
	// simply absent from the map. A error means some part of the lookup could
	// not be performed. Result map is still valid even if there's an error.
	Query(ctx context.Context, isins []string) (map[string]Info, error)
}

var _ Querier = chain{}

type chain struct{ aueriers []Querier }

// Chain asks queriers in order. Each is asked only about the ISINs that
// earlier ones could not describe. A querier that fails is skipped; the
// first error is returned together with the partial results.
func Chain(queriers ...Querier) Querier { return chain{aueriers: queriers} }

func (c chain) Name() string {
	names := make([]string, len(c.aueriers))
	for i, q := range c.aueriers {
		names[i] = q.Name()
	}
	return strings.Join(names, ">")
}

func (c chain) Query(ctx context.Context, isins []string) (map[string]Info, error) {
	found := map[string]Info{}
	var firstErr error
	for _, querier := range c.aueriers {
		var toQuery []string
		for _, isin := range isins {
			if !found[isin].known() {
				toQuery = append(toQuery, isin)
			}
		}
		if len(toQuery) == 0 {
			break
		}
		res, err := querier.Query(ctx, toQuery)
		for isin, info := range res {
			isin = strings.ToUpper(isin)
			found[isin] = merge(found[isin], info)
		}
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("%s: %w", querier.Name(), err)
		}
	}
	return found, firstErr
}

// merge fills zero fields of a from b.
func merge(a, b Info) Info {
	if a.CFI == "" {
		a.CFI = b.CFI
	}
	if a.Issuer == IssuerUnknown {
		a.Issuer = b.Issuer
	}
	if a.IssuerName == "" {
		a.IssuerName = b.IssuerName
	}
	return a
}
