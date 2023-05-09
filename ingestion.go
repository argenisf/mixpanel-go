package mixpanel

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
)

const (
	trackURL  = "/track"
	importURL = "/import"

	// People urls
	peopleSetURL            = "/engage#profile-set"
	peopleSetOnceURL        = "/engage#profile-set-once"
	peopleIncrementUrl      = "/engage#profile-numerical-add"
	peopleUnionToListUrl    = "/engage#profile-union"
	peopleAppendToListUrl   = "/engage#profile-list-append"
	peopleRemoveFromListUrl = "/engage#profile-list-remove"
	// peopleBatchUpdateUrl    = "/engage#profile-batch-update" todo implement, won't in v0
	peopleDeletePropertyUrl = "/engage#profile-unset"
	peopleDeleteProfileUrl  = "/engage#profile-delete"

	// Group urls
	groupSetUrl                     = "/groups#group-set"
	groupsSetOnceUrl                = "/groups#group-set-once"
	groupsDeletePropertyUrl         = "/groups#group-unset"
	groupsRemoveFromListPropertyUrl = "/groups#group-remove-from-list"
	groupsUnionListPropertyUrl      = "/groups#group-union"
	// groupsBatchGroupProfilesUrl     = "/groups#group-batch-update" todo implement, won't in v0
	groupsDeleteGroupUrl = "/groups#group-delete"

	// Lookup tables
	lookupTablesUrl = "/lookup-tables"
	//replaceLookupTableUrl = "/lookup-tables/" todo implement, won't in v0
)

// Track calls the Track endpoint
// For server side we recommend Import func
// more info here: https://developer.mixpanel.com/reference/track-event#when-to-use-track-vs-import
func (m *Mixpanel) Track(ctx context.Context, events []*Event) error {
	query := url.Values{}
	query.Add("verbose", "1")

	response, err := m.doRequest(
		ctx,
		http.MethodPost,
		m.apiEndpoint+trackURL,
		events,
		None,
		addQueryParams(query), acceptPlainText(),
	)
	if err != nil {
		return fmt.Errorf("failed to track event: %w", err)
	}
	defer response.Body.Close()

	return returnVerboseError(response)
}

type ImportFailedValidationError struct {
	Code                int                   `json:"code"`
	ApiError            string                `json:"error"`
	Status              interface{}           `json:"status"`
	NumRecordsImported  int                   `json:"num_records_imported"`
	FailedImportRecords []ImportFailedRecords `json:"failed_records"`
}

type ImportFailedRecords struct {
	Index    int    `json:"index"`
	InsertID int    `json:"insert_id"`
	Field    string `json:"field"`
	Message  string `json:"message"`
}

func (e ImportFailedValidationError) Error() string {
	return e.ApiError
}

type ImportOptions struct {
	Strict      bool
	Compression MpCompression
}

var ImportOptionsRecommend = ImportOptions{
	Strict:      true,
	Compression: Gzip,
}

type ImportSuccess struct {
	Code               int         `json:"code"`
	NumRecordsImported int         `json:"num_records_imported"`
	Status             interface{} `json:"status"`
}

type ImportGenericError struct {
	Code     int         `json:"code"`
	ApiError string      `json:"error"`
	Status   interface{} `json:"status"`
}

func (e ImportGenericError) Error() string {
	return e.ApiError
}

// Import calls the Import api
// https://developer.mixpanel.com/reference/import-events
func (m *Mixpanel) Import(ctx context.Context, events []*Event, options ImportOptions) (*ImportSuccess, error) {
	values := url.Values{}
	if options.Strict {
		values.Add("strict", "1")
	} else {
		values.Add("strict", "0")
	}
	values.Add("project_id", strconv.Itoa(m.projectID))
	values.Add("verbose", "1")

	httpResponse, err := m.doRequest(
		ctx,
		http.MethodPost,
		m.apiEndpoint+importURL,
		events,
		options.Compression,
		addQueryParams(values), acceptJson(), m.useServiceAccount(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to import:%w", err)
	}
	defer httpResponse.Body.Close()

	switch httpResponse.StatusCode {
	case http.StatusOK:
		var s ImportSuccess
		if err := json.NewDecoder(httpResponse.Body).Decode(&s); err != nil {
			return nil, fmt.Errorf("failed to parse response body:%w", err)
		}
		return &s, nil
	case http.StatusBadRequest:
		var g ImportFailedValidationError
		if err := json.NewDecoder(httpResponse.Body).Decode(&g); err != nil {
			return nil, fmt.Errorf("failed to json decode response body: %w", err)
		}
		return nil, g
	case http.StatusUnauthorized, http.StatusRequestEntityTooLarge, http.StatusTooManyRequests:
		var g ImportGenericError
		if err := json.NewDecoder(httpResponse.Body).Decode(&g); err != nil {
			return nil, fmt.Errorf("failed to json decode response body: %w", err)
		}
		return nil, g
	default:
		return nil, fmt.Errorf("unexpected status code: %d", httpResponse.StatusCode)
	}
}

// PeopleOptions
type peopleOptions struct {
	IP        string  `json:"$ip,omitempty"`
	Latitude  float64 `json:"$latitude,omitempty"`
	Longitude float64 `json:"$longitude,omitempty"`
}

type PeopleOptions func(p *peopleOptions)

func SetPeopleIP(ip net.IP) PeopleOptions {
	return func(p *peopleOptions) {
		p.IP = ip.String()
	}
}

func SetPeopleLatLong(lat, long float64) PeopleOptions {
	return func(p *peopleOptions) {
		p.Latitude = lat
		p.Longitude = long
	}
}

type peopleSetPayload struct {
	Token      string         `json:"$token"`
	DistinctID string         `json:"$distinct_id"`
	Set        map[string]any `json:"$set"`
	*peopleOptions
}

// PeopleSet calls the User Set Property API
// https://developer.mixpanel.com/reference/profile-set
func (m *Mixpanel) PeopleSet(ctx context.Context, distinctID string, properties map[string]any, options ...PeopleOptions) error {
	payload := peopleSetPayload{
		Token:         m.token,
		DistinctID:    distinctID,
		peopleOptions: &peopleOptions{},
		Set:           properties,
	}
	for _, o := range options {
		o(payload.peopleOptions)
	}
	return m.doPeopleRequest(ctx, []peopleSetPayload{payload}, peopleSetURL)
}

type peopleSetOncePayload struct {
	Token      string         `json:"$token"`
	DistinctID string         `json:"$distinct_id"`
	SetOnce    map[string]any `json:"$set_once"`
	*peopleOptions
}

// PeopleSetOnce calls the User Set Property Once API
// https://developer.mixpanel.com/reference/profile-set-property-once
func (m *Mixpanel) PeopleSetOnce(ctx context.Context, distinctID string, properties map[string]any, options ...PeopleOptions) error {
	payload := peopleSetOncePayload{
		Token:         m.token,
		DistinctID:    distinctID,
		SetOnce:       properties,
		peopleOptions: &peopleOptions{},
	}
	for _, o := range options {
		o(payload.peopleOptions)
	}
	return m.doPeopleRequest(ctx, []peopleSetOncePayload{payload}, peopleSetOnceURL)
}

type peopleNumericalAddPayload struct {
	Token      string         `json:"$token"`
	DistinctID string         `json:"$distinct_id"`
	Add        map[string]int `json:"$add"`
}

// PeopleIncrement calls the User Increment Numerical Property API
// https://developer.mixpanel.com/reference/profile-numerical-add
func (m *Mixpanel) PeopleIncrement(ctx context.Context, distinctID string, add map[string]int) error {
	payload := []peopleNumericalAddPayload{
		{
			Token:      m.token,
			DistinctID: distinctID,
			Add:        add,
		},
	}
	return m.doPeopleRequest(ctx, payload, peopleIncrementUrl)
}

type peopleUnionPayload struct {
	Token      string         `json:"$token"`
	DistinctID string         `json:"$distinct_id"`
	Union      map[string]any `json:"$union"`
}

// PeopleUnionProperty calls User Union To List Property API
// https://developer.mixpanel.com/reference/user-profile-union
func (m *Mixpanel) PeopleUnionProperty(ctx context.Context, distinctID string, union map[string]any) error {
	payload := []peopleUnionPayload{
		{
			Token:      m.token,
			DistinctID: distinctID,
			Union:      union,
		},
	}
	return m.doPeopleRequest(ctx, payload, peopleUnionToListUrl)
}

type peopleAppendListPayload struct {
	Token      string            `json:"$token"`
	DistinctID string            `json:"$distinct_id"`
	Append     map[string]string `json:"$append"`
}

// PeopleAppend calls the Increment Numerical Property
// https://developer.mixpanel.com/reference/profile-numerical-add
func (m *Mixpanel) PeopleAppendListProperty(ctx context.Context, distinctID string, append map[string]string) error {
	payload := []peopleAppendListPayload{
		{
			Token:      m.token,
			DistinctID: distinctID,
			Append:     append,
		},
	}
	return m.doPeopleRequest(ctx, payload, peopleAppendToListUrl)
}

type peopleListRemovePayload struct {
	Token      string            `json:"$token"`
	DistinctID string            `json:"$distinct_id"`
	Remove     map[string]string `json:"$remove"`
}

// PeopleRemoveListProperty calls the User Remove from List Property API
// https://developer.mixpanel.com/reference/profile-remove-from-list-property
func (m *Mixpanel) PeopleRemoveListProperty(ctx context.Context, distinctID string, remove map[string]string) error {
	payload := []peopleListRemovePayload{
		{
			Token:      m.token,
			DistinctID: distinctID,
			Remove:     remove,
		},
	}
	return m.doPeopleRequest(ctx, payload, peopleRemoveFromListUrl)
}

type peopleDeletePropertyPayload struct {
	Token      string   `json:"$token"`
	DistinctID string   `json:"$distinct_id"`
	Unset      []string `json:"$unset"`
}

// PeopleDeleteProperty calls the User Delete Property API
// https://developer.mixpanel.com/reference/profile-delete-property
func (m *Mixpanel) PeopleDeleteProperty(ctx context.Context, distinctID string, unset []string) error {
	payload := []peopleDeletePropertyPayload{
		{
			Token:      m.token,
			DistinctID: distinctID,
			Unset:      unset,
		},
	}
	return m.doPeopleRequest(ctx, payload, peopleDeletePropertyUrl)
}

type peopleDeleteProfilePayload struct {
	Token       string `json:"$token"`
	DistinctID  string `json:"$distinct_id"`
	Delete      string `json:"$delete"`
	IgnoreAlias string `json:"$ignore_alias"`
}

// PeopleDeleteProfile calls the User Delete Profile API
// https://developer.mixpanel.com/reference/delete-profile
func (m *Mixpanel) PeopleDeleteProfile(ctx context.Context, distinctID string, ignoreAlias bool) error {
	payload := []peopleDeleteProfilePayload{
		{
			Token:       m.token,
			DistinctID:  distinctID,
			Delete:      "null", // The $delete object value is ignored - the profile is determined by the $distinct_id from the request itself.
			IgnoreAlias: strconv.FormatBool(ignoreAlias),
		},
	}
	return m.doPeopleRequest(ctx, payload, peopleDeleteProfileUrl)
}

type groupUpdatePropertyPayload struct {
	Token    string            `json:"$token"`
	GroupKey string            `json:"$group_key"`
	GroupId  string            `json:"$group_id"`
	Set      map[string]string `json:"$set"`
}

// GroupUpdateProperty calls the Group Update Property API
// https://developer.mixpanel.com/reference/group-set-property
func (m *Mixpanel) GroupUpdateProperty(ctx context.Context, groupKey, groupID string, set map[string]string) error {
	payload := []groupUpdatePropertyPayload{
		{
			Token:    m.token,
			GroupKey: groupKey,
			GroupId:  groupID,
			Set:      set,
		},
	}
	return m.doPeopleRequest(ctx, payload, groupSetUrl)
}

type groupSetOncePropertyPayload struct {
	Token    string         `json:"$token"`
	GroupKey string         `json:"$group_key"`
	GroupId  string         `json:"$group_id"`
	SetOnce  map[string]any `json:"$set_once"`
}

// GroupSetOnce calls the Group Set Property Once API
// https://developer.mixpanel.com/reference/group-set-property-once
func (m *Mixpanel) GroupSetOnce(ctx context.Context, groupKey, groupID string, set map[string]any) error {
	payload := []groupSetOncePropertyPayload{
		{
			Token:    m.token,
			GroupKey: groupKey,
			GroupId:  groupID,
			SetOnce:  set,
		},
	}
	return m.doPeopleRequest(ctx, payload, groupsSetOnceUrl)
}

type groupDeletePropertyPayload struct {
	Token    string   `json:"$token"`
	GroupKey string   `json:"$group_key"`
	GroupId  string   `json:"$group_id"`
	Unset    []string `json:"$unset"`
}

// GroupDeleteProperty calls the group delete property API
// https://developer.mixpanel.com/reference/group-delete-property
func (m *Mixpanel) GroupDeleteProperty(ctx context.Context, groupKey, groupID string, unset []string) error {
	payload := []groupDeletePropertyPayload{
		{
			Token:    m.token,
			GroupKey: groupKey,
			GroupId:  groupID,
			Unset:    unset,
		},
	}
	return m.doPeopleRequest(ctx, payload, groupsDeletePropertyUrl)
}

type groupRemoveListPropertyPayload struct {
	Token    string            `json:"$token"`
	GroupKey string            `json:"$group_key"`
	GroupId  string            `json:"$group_id"`
	Remove   map[string]string `json:"$remove"`
}

// GroupRemoveListProperty calls the Groups Remove from List Property API
// https://developer.mixpanel.com/reference/group-remove-from-list-property
func (m *Mixpanel) GroupRemoveListProperty(ctx context.Context, groupKey, groupID string, remove map[string]string) error {
	payload := []groupRemoveListPropertyPayload{
		{
			Token:    m.token,
			GroupKey: groupKey,
			GroupId:  groupID,
			Remove:   remove,
		},
	}
	return m.doPeopleRequest(ctx, payload, groupsRemoveFromListPropertyUrl)
}

type groupUnionListPropertyPayload struct {
	Token    string         `json:"$token"`
	GroupKey string         `json:"$group_key"`
	GroupId  string         `json:"$group_id"`
	Union    map[string]any `json:"$union"`
}

// GroupUnionListProperty calls the Groups Remove from Union Property API
// https://developer.mixpanel.com/reference/group-union
func (m *Mixpanel) GroupUnionListProperty(ctx context.Context, groupKey, groupID string, union map[string]any) error {
	payload := []groupUnionListPropertyPayload{
		{
			Token:    m.token,
			GroupKey: groupKey,
			GroupId:  groupID,
			Union:    union,
		},
	}
	return m.doPeopleRequest(ctx, payload, groupsUnionListPropertyUrl)
}

type groupDeletePayload struct {
	Token    string `json:"$token"`
	GroupKey string `json:"$group_key"`
	GroupId  string `json:"$group_id"`
	Delete   string `json:"$delete"`
}

// GroupDelete calls the Groups Delete API
// https://developer.mixpanel.com/reference/delete-group
func (m *Mixpanel) GroupDelete(ctx context.Context, groupKey, groupID string) error {
	payload := []groupDeletePayload{
		{
			Token:    m.token,
			GroupKey: groupKey,
			GroupId:  groupID,
			Delete:   "null",
		},
	}

	return m.doPeopleRequest(ctx, payload, groupsDeleteGroupUrl)
}

type LookupTable struct {
	Code    int                  `json:"code"`
	Status  string               `json:"status"`
	Results []LookupTableResults `json:"results"`
}

type LookupTableResults struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type LookupTableError struct {
	Code     int         `json:"code"`
	ApiError string      `json:"error"`
	Status   interface{} `json:"status"`
}

func (e LookupTableError) Error() string {
	return e.ApiError
}

// ListLookupTables calls the List Lookup Tables API
// https://developer.mixpanel.com/reference/list-lookup-tables
func (m *Mixpanel) ListLookupTables(ctx context.Context) (*LookupTable, error) {
	query := url.Values{}
	query.Add("project_id", strconv.Itoa(m.projectID))

	httpResponse, err := m.doRequest(
		ctx,
		http.MethodGet,
		m.apiEndpoint+lookupTablesUrl,
		nil,
		None,
		addQueryParams(query), acceptJson(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call list lookup tables: %v", err)
	}

	defer httpResponse.Body.Close()

	switch httpResponse.StatusCode {
	case http.StatusOK:
		var result LookupTable
		if err := json.NewDecoder(httpResponse.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode results: %w", err)
		}
		return &result, nil
	case http.StatusUnauthorized:
		var e LookupTableError
		if err := json.NewDecoder(httpResponse.Body).Decode(&e); err != nil {
			return nil, fmt.Errorf("failed to decode error response:%w", err)
		}
		return nil, e
	default:
		return nil, fmt.Errorf("unexpected status code: %d", httpResponse.StatusCode)
	}
}