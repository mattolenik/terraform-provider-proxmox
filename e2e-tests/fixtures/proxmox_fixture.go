package fixtures

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// ProxmoxTestFixture is a test helper for bringing up Vagrant VMs that run Proxmox.
type ProxmoxTestFixture struct {
	BaseFixture
	VagrantTestFixture
	// The Vagrant provider to use, defaults to virtualbox
	VagrantProvider string
	// Name is a descriptive name for this test fixture.
	Name string
	// URL of Proxmox instance
	Endpoint     string
	httpClient   *http.Client
	testUsername string
	testPassword string
	ticket       string
}

// NewProxmoxTestFixture creates a new Vagrant-based test fixture for working with Proxmox.
// Calling this function will asynchronously bring up a VM for running Proxmox.
func NewProxmoxTestFixture(t *testing.T, vagrantProvider, proxmoxEndpoint, name, testUsername, testPassword string) chan *ProxmoxTestFixture {
	base := NewBaseFixture(t)
	c := make(chan *ProxmoxTestFixture, 1)
	func() {
		f := &ProxmoxTestFixture{
			BaseFixture:        base,
			VagrantTestFixture: NewVagrantTestFixture(vagrantProvider),
			VagrantProvider:    vagrantProvider,
			Name:               name,
			Endpoint:           proxmoxEndpoint,
			httpClient:         http.DefaultClient,
			testUsername:       testUsername,
			testPassword:       testPassword,
		}
		f.start()
		c <- f
	}()
	return c
}

// start brings up the Proxmox VM
func (f *ProxmoxTestFixture) start() {
	// Bring up the VM
	err := f.Up()
	f.Require.NoErrorf(err, "failed to bring up VM for fixture '%s'", f.Name)
}

// TearDown removes every trace the test fixture.
// It should be called with defer right after creating the fixture.
func (f *ProxmoxTestFixture) TearDown() {
	if !f.ShouldClean(f) {
		return
	}
	// Turn off the VM.
	err := f.Halt()
	f.Assert.NoErrorf(err, "failed shutting down VM for fixture '%s'", f.Name)
}

func (f *ProxmoxTestFixture) initTicket() {
	if f.ticket != "" {
		return
	}
	reqBody := fmt.Sprintf("username=%s&password=%s", f.testUsername, f.testPassword)
	ticketURL, err := url.Parse(f.Endpoint + "/access/ticket")
	f.Require.NoError(err, "Failed trying to parse ticket URL")
	resp, err := f.httpClient.Do(&http.Request{
		Method: "GET",
		URL:    ticketURL,
		Body:   io.NopCloser(bytes.NewBuffer([]byte(reqBody))),
	})
	f.Require.NoError(err, "Failed trying to get ticket")
	respBody, err := io.ReadAll(resp.Body)
	f.Require.NoError(err, "Failed trying to read ticket response")

	// Quick anonymous struct for exracting auth ticket
	respStruct := struct {
		Data struct {
			Ticket string `json:"ticket"`
		} `json:"data"`
	}{}
	err = json.Unmarshal(respBody, &respStruct)
	f.Require.NoError(err, "Failed trying to unmarshal ticket response")
	f.ticket = respStruct.Data.Ticket
}

func (f *ProxmoxTestFixture) APIGet(apiName string) map[string]interface{} {
	f.initTicket()
	params := fmt.Sprintf("?username=%s&password=%s", f.testUsername, f.testPassword)
	url, err := url.Parse(f.Endpoint + "/" + apiName + params)
	f.Require.NoErrorf(err, "Invalid API name, should be in the form of e.g. access/roles")
	resp, err := f.httpClient.Do(&http.Request{
		Method: "GET",
		URL:    url,
		Header: http.Header{"PVEAuthCookie": []string{f.ticket}},
	})
	f.Require.NoErrorf(err, "Unexpected error when performing HTTP GET on '%s'", url.String())
	jsonBody, err := ioutil.ReadAll(resp.Body)
	f.Require.NoErrorf(err, "Unexpected error when reading response from '%s'", url.String())
	var jsonObj map[string]interface{}
	err = json.Unmarshal(jsonBody, &jsonObj)
	f.Require.NoErrorf(err, "Unexpected error when unmarshaling JSON from '%s'", url.String())
	return jsonObj
}
