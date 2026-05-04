package subscription

import "time"

// Subscription is the persisted shape of a VPN subscription.
type Subscription struct {
	ID             string    `json:"id"`                      // uuid
	Label          string    `json:"label"`                   // user-facing
	URL            string    `json:"url"`                     // subscription URL
	Headers        []Header  `json:"headers"`                 // custom HTTP headers for fetch
	RefreshHours   int       `json:"refreshHours"`            // 0 = manual only
	LastFetched    time.Time `json:"lastFetched"`
	LastError      string    `json:"lastError,omitempty"`
	SelectorTag    string    `json:"selectorTag"`             // "sub-<id-short>"
	InboundTag     string    `json:"inboundTag"`              // "sub-<id-short>-in"
	ListenPort     uint16    `json:"listenPort"`              // localhost port for the mixed inbound
	MemberTags     []string  `json:"memberTags"`              // every member outbound tag
	OrphanTags     []string  `json:"orphanTags"`              // tags missing on last refresh
	ActiveMember   string    `json:"activeMember,omitempty"` // currently-active selector member tag
	Enabled        bool      `json:"enabled"`
	IsDefaultRoute bool      `json:"isDefaultRoute"`
}

// Header is a single name:value pair sent on the fetch request.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CreateInput is the input to Service.Create.
type CreateInput struct {
	Label        string
	URL          string
	Headers      []Header
	RefreshHours int
	Enabled      bool
}

// UpdatePatch is partial update; nil pointers mean "leave as-is".
type UpdatePatch struct {
	Label        *string
	URL          *string
	Headers      *[]Header
	RefreshHours *int
	Enabled      *bool
}

// RefreshResult is the outcome of a single refresh cycle.
type RefreshResult struct {
	When         time.Time
	Err          error
	Added        int
	Updated      int
	Orphaned     int
	SkippedVmess int
	SkippedOther int
	ParseErrors  []string
}
