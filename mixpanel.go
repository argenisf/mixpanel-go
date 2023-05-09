package mixpanel

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

const (
	version = "v0.0.1"

	usEndpoint     = "https://api.mixpanel.com"
	usDataEndpoint = "https://data.mixpanel.com"

	euEndpoint     = "https://api-eu.mixpanel.com"
	euDataEndpoint = "https://data-eu.mixpanel"

	EmptyDistinctID = ""

	propertyToken      = "token"
	propertyDistinctID = "distinct_id"

	propertyIP         = "ip"
	propertyInsertID   = "$insert_id"
	propertyTime       = "time"
	propertyMpLib      = "mp_lib"
	goLib              = "go"
	propertyLibVersion = "$lib_version"

	acceptHeader          = "Accept"
	acceptPlainTextHeader = "text/plain"
	acceptJsonHeader      = "application/json"
	contentEncodingHeader = "Content-Encoding"
)

type MpCompression int

var (
	None MpCompression = 0
	Gzip MpCompression = 1
)

type Ingestion interface {
	// Events
	Track(ctx context.Context, events []*Event) error
	Import(ctx context.Context, events []*Event, options ImportOptions) (*ImportSuccess, error)

	// People
	PeopleSet(ctx context.Context, distinctID string, properties map[string]any, options ...PeopleOptions) error
	PeopleSetOnce(ctx context.Context, distinctID string, properties map[string]any, options ...PeopleOptions) error
	PeopleIncrement(ctx context.Context, distinctID string, add map[string]int) error
	PeopleUnionProperty(ctx context.Context, distinctID string, union map[string]any) error
	PeopleAppendListProperty(ctx context.Context, distinctID string, append map[string]string) error
	PeopleRemoveListProperty(ctx context.Context, distinctID string, remove map[string]string) error
	PeopleDeleteProperty(ctx context.Context, distinctID string, unset []string) error
	PeopleDeleteProfile(ctx context.Context, distinctID string, ignoreAlias bool) error

	// Groups
	GroupUpdateProperty(ctx context.Context, groupKey, groupID string, set map[string]string) error
	GroupSetOnce(ctx context.Context, groupKey, groupID string, set map[string]any) error
	GroupDeleteProperty(ctx context.Context, groupKey, groupID string, unset []string) error
	GroupRemoveListProperty(ctx context.Context, groupKey, groupID string, remove map[string]string) error
	GroupUnionListProperty(ctx context.Context, groupKey, groupID string, union map[string]any) error
	GroupDelete(ctx context.Context, groupKey, groupID string) error

	// Lookup
	ListLookupTables(ctx context.Context) (*LookupTable, error)
}

var _ Ingestion = (*Mixpanel)(nil)

type Export interface {
	Export(ctx context.Context, fromDate, toDate time.Time, limit int, event, where string) ([]*Event, error)
}

var _ Export = (*Mixpanel)(nil)

type Identity interface {
	CreateIdentity(ctx context.Context, events []*IdentityEvent, options IdentityOptions) error
}

var _ Identity = (*Mixpanel)(nil)

// MpApi is all the API's in the Mixpanel docs
// https://developer.mixpanel.com/reference/overview
type MpApi interface {
	Ingestion
}

type ServiceAccount struct {
	Username string
	Secret   string
}

type Mixpanel struct {
	client       *http.Client
	apiEndpoint  string
	dataEndpoint string

	projectID int
	token     string
	apiSecret string

	serviceAccount *ServiceAccount

	debugHttp bool
}

type Options func(mixpanel *Mixpanel)

// HttpClient will replace the http.DefaultClient with the provided http.Client
func HttpClient(client *http.Client) Options {
	return func(mixpanel *Mixpanel) {
		mixpanel.client = client
	}
}

// EuResidency sets the mixpanel client to use the eu endpoints
// Use for EU Projects
func EuResidency() Options {
	return func(mixpanel *Mixpanel) {
		mixpanel.apiEndpoint = euEndpoint
		mixpanel.dataEndpoint = euDataEndpoint
	}
}

// ProxyApiLocation sets the mixpanel client to use the custom location for all ingestion requests
// Example: http://locahosthost:8080
func ProxyApiLocation(proxy string) Options {
	return func(mixpanel *Mixpanel) {
		mixpanel.apiEndpoint = proxy
	}
}

// ProxyDataLocation sets the mixpanel client to use the custom location for all data requests
// Example: http://locahosthost:8080
func ProxyDataLocation(proxy string) Options {
	return func(mixpanel *Mixpanel) {
		mixpanel.dataEndpoint = proxy
	}
}

// SetServiceAccount add a service account to the mixpanel client
// https://developer.mixpanel.com/reference/service-accounts-api
func SetServiceAccount(username, secret string) Options {
	return func(mixpanel *Mixpanel) {
		mixpanel.serviceAccount = &ServiceAccount{
			Username: username,
			Secret:   secret,
		}
	}
}

// DebugHttpCalls prints payload information and url information for debugging purposes
func DebugHttpCalls() Options {
	return func(mixpanel *Mixpanel) {
		mixpanel.debugHttp = true
	}
}

// NewClient create a new mixpanel client
func NewClient(projectID int, token, secret string, options ...Options) *Mixpanel {
	mp := &Mixpanel{
		projectID:    projectID,
		client:       http.DefaultClient,
		apiEndpoint:  usEndpoint,
		dataEndpoint: usDataEndpoint,
		token:        token,
		apiSecret:    secret,
	}

	for _, o := range options {
		o(mp)
	}

	return mp
}

// Event is a mixpanel event: https://help.mixpanel.com/hc/en-us/articles/360041995352-Mixpanel-Concepts-Events
type Event struct {
	Name       string         `json:"event"`
	Properties map[string]any `json:"properties"`
}

// NewEvent creates a new mixpanel event to track
func (m *Mixpanel) NewEvent(name string, distinctID string, properties map[string]any) *Event {
	e := &Event{
		Name: name,
	}
	if properties == nil {
		properties = make(map[string]any)
	}

	// Todo: deep copy map
	properties[propertyToken] = m.token
	properties[propertyDistinctID] = distinctID
	properties[propertyMpLib] = goLib
	properties[propertyLibVersion] = version
	e.Properties = properties

	return e
}

func (m *Mixpanel) NewEventFromJson(json map[string]any) (*Event, error) {
	name, ok := json["event"].(string)
	if !ok {
		return nil, errors.New("event name is not a string or is missing")
	}

	properties, ok := json["properties"].(map[string]any)
	if !ok {
		return nil, errors.New("event properties is not a map or is missing")
	}

	return &Event{
		Name:       name,
		Properties: properties,
	}, nil
}

// AddTime insert the time properties into the event
// https://developer.mixpanel.com/reference/import-events#propertiestime
func (e *Event) AddTime(t time.Time) {
	e.Properties[propertyTime] = t.UnixMilli()
}

// AddInsertID inserts the insert_id property into the properties
// https://developer.mixpanel.com/reference/import-events#propertiesinsert_id
func (e *Event) AddInsertID(insertID string) {
	e.Properties[propertyInsertID] = insertID
}

// AddIP if you supply a property ip with an IP address
// Mixpanel will automatically do a GeoIP lookup and replace the ip property with geographic properties (City, Country, Region). These properties can be used in our UI to segment events geographically.
// https://developer.mixpanel.com/reference/import-events#geoip-enrichment
func (e *Event) AddIP(ip *net.IP) {
	e.Properties[propertyIP] = ip.String()
}

type IdentityEvent struct {
	*Event
}

func (m *Mixpanel) NewIdentityEvent(distinctID string, properties map[string]any, identifiedId, anonId string) *IdentityEvent {
	event := m.NewEvent("$identify", distinctID, properties)
	i := &IdentityEvent{
		Event: event,
	}
	i.SetIdentifiedId(identifiedId)
	i.SetAnonId(anonId)

	return i
}

func (i *IdentityEvent) IdentifiedId() any {
	return i.Properties["$identified_id"]
}

// A distinct_id to merge with the $anon_id.
func (i *IdentityEvent) SetIdentifiedId(id string) {
	i.Properties["$identified_id"] = id
}

// A distinct_id to merge with the $identified_id. The $anon_id must be UUID v4 format and not already merged to an $identified_id.
func (i *IdentityEvent) SetAnonId(id string) {
	i.Properties["$anon_id"] = id
}

func (i *IdentityEvent) AnonId(id string) {
	i.Properties["$anon_id"] = id
}