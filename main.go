package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	origLog "log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	FeatureNone = iota
	FeatureSelect
	FeatureDelayTest
)

var in = bufio.NewReader(os.Stdin)
var log = origLog.New(os.Stderr, "", 0)

type Config struct {
	Port    *int // Nullable
	Addr    string
	Scheme  string
	Groups  []string
	TestURL string
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			`Usage of %[1]s:
            
Specify environment variables to control which Clash instance to control and which groups to select from.
Environment variables will be overridden by command line arguments, flags and options.

    CLASH_PORT          Clash external controller port. If not specified,
                        9090, 9091, 19090, 19091 will be tried sequen-
                        tially.
    CLASH_ADDR          Clash external controller address. If not
                        specified, defaults to 127.0.0.1.
    CLASH_SCHEME        Clash external controller scheme. If not
                        specified, defaults to http.
    CLASH_GROUPS        Which groups to select from. Can be group names
                        or group indexes (starts from 0), separated by
                        commas. E.g. "My Proxy,Video Media,3".
    CLASH_TEST_URL      Delay test URL. Defaults to
                        connectivitycheck.gstatic.com/generate_204 .

Command line:
    %[1]s [-h|--help]
    %[1]s [-p <port>] [-a <addr>] [-u <url>] [-e <scheme>] [-s|-t]
            [<Group1> [<Group2> [<G3> ...]]]

`, os.Args[0])
		flag.PrintDefaults()
	}

	var portFlag = flag.Int("p", -1, "Clash external controller port")
	var addrFlag = flag.String("a", "", "Clash external controller address")
	var schemeFlag = flag.String("e", "", "Clash external controller scheme")
	var testURLFlag = flag.String("u", "", "Delay test URL")
	var selectFlag = flag.Bool("s", false, "(Select) Use node select feature. This is the default feature")
	var delayTestFlag = flag.Bool("t", false, "(delay Test) Use delay test feature. You can specify only 1 proxy group in this case")

	flag.Parse()

	if *portFlag != -1 && (*portFlag <= 0 || *portFlag > 65535) {
		log.Panicf("Bad port: %d\n", *portFlag)
	}
	port := portFlag
	if *port == -1 {
		portStr := os.Getenv("CLASH_PORT")
		if portStr != "" {
			var err error
			*port, err = strconv.Atoi(portStr)
			if err != nil || *port <= 0 || *port > 65535 {
				log.Panicf("Bad port: %s\n", portStr)
			}
		} else {
			port = nil // Try 9090, 9091, ...
		}
	}

	addr := *addrFlag
	if addr == "" {
		addr = os.Getenv("CLASH_ADDR")
		if addr == "" {
			addr = "127.0.0.1"
		}
	}

	scheme := *schemeFlag
	if scheme == "" {
		scheme = os.Getenv("CLASH_SCHEME")
		if scheme == "" {
			scheme = "http"
		}
	}
	scheme = strings.ToLower(scheme)
	if scheme != "http" && scheme != "https" {
		log.Panicf("Unsupported scheme: %s\n", scheme)
	}

	testURL := *testURLFlag
	if testURL == "" {
		testURL = os.Getenv("CLASH_TEST_URL")
		if testURL == "" {
			testURL = "http://connectivitycheck.gstatic.com/generate_204"
		}
	}

	feature := FeatureNone
	featureNum := 0
	if *selectFlag {
		featureNum++
		feature = FeatureSelect
	}

	if *delayTestFlag {
		featureNum++
		feature = FeatureDelayTest
	}

	if featureNum > 1 {
		log.Panicf("Can't select more than one feature")
	}

	if feature == FeatureNone {
		feature = FeatureSelect
	}

	groups := flag.Args()
	if len(groups) == 0 {
		groupsEnv := strings.Split(os.Getenv("CLASH_GROUPS"), ",")
		groups = make([]string, 0)
		for _, v := range groupsEnv {
			s := strings.TrimSpace(v)
			if s != "" {
				groups = append(groups, s)
			}
		}
	}

	config := Config{
		Port:    port,
		Addr:    addr,
		Scheme:  scheme,
		Groups:  groups,
		TestURL: testURL,
	}

	portPrint := "<Not decided>"
	if port != nil {
		portPrint = fmt.Sprintf("%d", *port)
	}

	fmt.Printf(
		`Using:
    Clash external controller: %s://%s:%s
    Groups: %v
    TestURL: %s

`,
		scheme,
		addr,
		portPrint,
		groups,
		testURL,
	)

	switch feature {
	case FeatureSelect:
		fmt.Println("> Selecting Nodes")
		fmt.Println()
		doSelectNode(&config)
	case FeatureDelayTest:
		fmt.Println("> Doing Delay Test")
		fmt.Println()
		doDelayTest(&config)
	}
}

type (
	ClashProxyOrGroup struct {
		Name string   `json:"name"`
		All  []string `json:"all"`
		Now  string   `json:"now"`
		Type string   `json:"type"`
	}

	ClashProxiesResponse struct {
		ProxiesAndGroups map[string]ClashProxyOrGroup `json:"proxies"`
	}

	ClashDelayTestResponse struct {
		Delay   int    `json:"delay"`
		Message string `json:"message"`
	}

	ClashSelectNodeRequest struct {
		Name string `json:"name"`
	}
)

func (p *ClashProxyOrGroup) isGroup() bool {
	return p.Type == "Selector"
}

func decidePort(config *Config) (int, error) {
	if config.Port != nil {
		return *config.Port, nil
	}

	c := &http.Client{
		Timeout: 300 * time.Millisecond,
	}
	for _, p := range []int{9090, 9091, 19090, 19091} {
		fmt.Printf("Trying port %d...", p) // In go, fmt.Print* functions are not buffered

		r, err := c.Get(fmt.Sprintf("%s://%s:%d/", config.Scheme, config.Addr, p))
		if err != nil {
			fmt.Printf("FAIL(Response): %s\n", err.Error())
			continue
		}

		m := make(map[string]interface{})
		err = json.NewDecoder(r.Body).Decode(&m)
		if err != nil {
			fmt.Printf("FAIL(Decoding): %s\n", err.Error())
			continue
		}

		v, ok := m["hello"]
		if !ok || v != "clash" {
			fmt.Printf("FAIL: not a Clash controller instance\n")
			continue
		}

		fmt.Println("OK")
		fmt.Println()
		return p, nil
	}
	return 0, errors.New("can't find a port that a Clash controller instance runs on")
}

func apiNewClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
	}
}

func apiGetGroups(baseURL string) ([]ClashProxyOrGroup, map[string]ClashProxyOrGroup, error) {
	c := apiNewClient()
	r, err := c.Get(baseURL + "/proxies")
	if err != nil {
		return nil, nil, err
	}
	if r.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("in apiGetGroups, got non-200 HTTP status code: %d", r.StatusCode)
	}

	pr := new(ClashProxiesResponse)
	err = json.NewDecoder(r.Body).Decode(&pr)
	if err != nil {
		return nil, nil, err
	}

	l := make([](ClashProxyOrGroup), 0)
	if globalGroup, ok := pr.ProxiesAndGroups["GLOBAL"]; ok {
		for _, v := range globalGroup.All {
			if currProxyOrGroup, ok := pr.ProxiesAndGroups[v]; ok && currProxyOrGroup.isGroup() {
				l = append(l, currProxyOrGroup)
			}
		}
	} else {
		for _, v := range pr.ProxiesAndGroups {
			l = append(l, v)
		}
	}

	return l, pr.ProxiesAndGroups, nil
}

func apiSelectNode(baseURL string, groupName string, proxyName string) error {
	c := apiNewClient()

	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(ClashSelectNodeRequest{
		Name: proxyName,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(
		http.MethodPut,
		baseURL+"/proxies/"+url.PathEscape(groupName),
		b,
	)
	if err != nil {
		return err
	}

	r, err := c.Do(req)
	if err != nil {
		return err
	}

	if r.StatusCode != http.StatusNoContent {
		return fmt.Errorf("in apiSelectNode, return status should be 204, but got %d", r.StatusCode)
	}
	return nil
}

func apiDelayTest(baseURL string, proxyName string, testURL string, timeoutMillisec int) (int, error) {
	c := apiNewClient()
	c.Timeout = 120 * time.Second

	req, err := http.NewRequest(
		http.MethodGet,
		baseURL+"/proxies/"+url.PathEscape(proxyName)+"/delay",
		nil,
	)
	if err != nil {
		return -1, err
	}

	q := req.URL.Query()
	q.Add("timeout", fmt.Sprintf("%d", timeoutMillisec))
	q.Add("url", testURL)
	req.URL.RawQuery = q.Encode()

	r, err := c.Do(req)
	if err != nil {
		return -1, err
	}

	d := new(ClashDelayTestResponse)
	err = json.NewDecoder(r.Body).Decode(&d)
	if err != nil {
		return -1, err
	}

	if r.StatusCode == http.StatusOK {
		return d.Delay, nil
	} else {
		return d.Delay, fmt.Errorf("delay test error, status code %d, message \"%s\"", r.StatusCode, d.Message)
	}
}

func mustDecideBaseURL(config *Config) string {
	port, err := decidePort(config)
	if err != nil {
		panic(err)
	}

	return fmt.Sprintf("%s://%s:%d", config.Scheme, config.Addr, port)
}

func mustGetNonEmptyValidGroupNames(config *Config, groups []ClashProxyOrGroup, nameToProxyOrGroup map[string]ClashProxyOrGroup) []string {
	validInputGroupNames := make([]string, 0)
	for _, groupNameOrIndex := range config.Groups {
		if groupOrProxy, ok := nameToProxyOrGroup[groupNameOrIndex]; ok && groupOrProxy.isGroup() {
			validInputGroupNames = append(validInputGroupNames, groupNameOrIndex)
		} else if i, err := strconv.Atoi(groupNameOrIndex); err == nil && i >= 0 && i < len(groups) {
			validInputGroupNames = append(validInputGroupNames, groups[i].Name)
		}
	}
	if len(validInputGroupNames) != 0 {
		return validInputGroupNames
	}

	if len(config.Groups) != 0 {
		panic(errors.New("no input group names match those from Clash controller"))
	} else {
		// Prompt user for input
		for i, g := range groups {
			fmt.Printf("%d.\t%s Now: [%s]\n", i, g.Name, g.Now)
		}
		for {
			fmt.Printf("\nSelect group: [Group name/Index] ")
			line, err := in.ReadString('\n')
			if err != nil {
				panic(err)
			}
			line = strings.TrimSpace(line)
			if line == "" {
				fmt.Println("You must specify a group.")
				continue
			}
			if _, ok := nameToProxyOrGroup[line]; ok {
				validInputGroupNames = []string{line}
			} else if i, err := strconv.Atoi(line); err == nil && i >= 0 && i < len(groups) {
				validInputGroupNames = []string{groups[i].Name}
			} else {
				fmt.Println("Bad input.")
				continue
			}
			break
		}
	}

	return validInputGroupNames
}

func askUserForNode(prompt string, nameToProxyOrGroup map[string]ClashProxyOrGroup, currGroup *ClashProxyOrGroup, optional bool) string {
	var userSelected string = ""
	for {
		fmt.Printf("%s: [Node name/Index] ", prompt)
		line, err := in.ReadString('\n')
		if err != nil {
			panic(err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			if optional {
				return ""
			}
			fmt.Println("You must specify a node.")
			continue
		}
		if _, ok := nameToProxyOrGroup[line]; ok {
			userSelected = line
		} else if i, err := strconv.Atoi(line); err == nil && i >= 0 && i < len(currGroup.All) {
			userSelected = currGroup.All[i]
		} else {
			fmt.Println("Bad input.")
			continue
		}
		break
	}
	return userSelected
}

func doSelectNode(config *Config) {
	baseURL := mustDecideBaseURL(config)
	groups, nameToProxyOrGroup, err := apiGetGroups(baseURL)
	if err != nil {
		panic(err)
	}
	validInputGroupNames := mustGetNonEmptyValidGroupNames(config, groups, nameToProxyOrGroup)

	for _, g := range validInputGroupNames {
		currGroup, ok := nameToProxyOrGroup[g]
		if !ok {
			panic(fmt.Errorf("%s not in currGroup", g))
		}
		if !currGroup.isGroup() {
			panic(fmt.Errorf("currGroup.isGroup() is false"))
		}
		fmt.Printf("[Group %s]\n", g)
		for i, p := range currGroup.All {
			fmt.Printf("%d.\t%s\n", i, p)
		}
		fmt.Printf("\nCurrent group: %s\n", g)
		nowIndex := -1
		for i, v := range currGroup.All {
			if v == currGroup.Now {
				nowIndex = i
				break
			}
		}
		fmt.Printf("Currently selected: %d. %s\n\n", nowIndex, currGroup.Now)

		userSelected := askUserForNode("Select a node", nameToProxyOrGroup, &currGroup, true)
		if userSelected == "" {
			fmt.Println("Not selecting for this group.")
			continue
		}

		fmt.Printf("Selecting %s for group %s...", userSelected, g)
		err := apiSelectNode(baseURL, g, userSelected)
		if err != nil {
			fmt.Printf("FAIL: %s\n", err.Error())
			log.Printf("\nStop, because error encountered: %s\n", err.Error())
			return
		}
		fmt.Println("OK")
		fmt.Println()
	}
}

func doDelayTest(config *Config) {
	baseURL := mustDecideBaseURL(config)
	groups, nameToProxyOrGroup, err := apiGetGroups(baseURL)
	if err != nil {
		panic(err)
	}
	validInputGroupNames := mustGetNonEmptyValidGroupNames(config, groups, nameToProxyOrGroup)
	if len(validInputGroupNames) > 1 {
		fmt.Println("Only one group allowed when you are doing delay test. Picking the first one")
	}

	fmt.Println()

	g := validInputGroupNames[0]
	currGroup, ok := nameToProxyOrGroup[g]
	if !ok {
		panic(fmt.Errorf("%s not in currGroup", g))
	}
	fmt.Printf("[Group %s]\n", g)
	for i, p := range currGroup.All {
		fmt.Printf("%d.\t%s\n", i, p)
	}

	userSelected := askUserForNode("\nSelect a node to test", nameToProxyOrGroup, &currGroup, false)

	fmt.Printf("Testing %s...", userSelected)
	delay, err := apiDelayTest(baseURL, userSelected, config.TestURL, 5000)
	if err != nil {
		fmt.Printf("FAIL: %s\n", err.Error())
		log.Printf("\nStop, because error encountered: %s\n", err.Error())
		return
	}
	fmt.Printf("%d ms\n", delay)
}
