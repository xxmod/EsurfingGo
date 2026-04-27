package network

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"esurfing/model"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"esurfing/utils"
)

const (
	captiveURL     = "http://connect.rom.miui.com/generate_204"
	portalStartTag = "<!--//config.campus.js.chinatelecom.com"
	portalEndTag   = "//config.campus.js.chinatelecom.com-->"
	authKey        = "Eshore!@#"
)

// ConnectivityStatus represents the network connectivity status.
type ConnectivityStatus int

const (
	StatusSuccess ConnectivityStatus = iota
	StatusRequireAuthorization
	StatusRequestError
)

// ConfigResult holds the parsed portal config.
type ConfigResult struct {
	Status      ConnectivityStatus
	AuthURL     string
	TicketURL   string
	UserIP      string
	AcIP        string
	SchoolID    string
	Domain      string
	Area        string
	ExtraCfgURL map[string]string
}

// DetectConfig performs captive portal detection and parses configuration.
// verbose controls whether detailed diagnostic logs are printed (set true on status changes).
func DetectConfig(state StateProvider, verbose bool) ConfigResult {
	return DetectConfigWithClient(nil, state, verbose)
}

// DetectConfigWithClient performs captive portal detection and parses configuration.
// If httpClient is nil, a default client will be created.
func DetectConfigWithClient(httpClient *http.Client, state StateProvider, verbose bool) ConfigResult {
	if verbose {
		log.Printf("[DetectConfig] Starting captive portal detection, URL: %s", captiveURL)
		log.Printf("[DetectConfig] Client-ID: %s", state.GetClientID())
	}

	client := httpClient
	if client == nil {
		client = NewHTTPClient(state)
	}

	// Fetch the initial URL, then follow JS redirects if needed
	currentURL := captiveURL
	var resp *http.Response
	var content string
	var portalConfig string

	const maxAttempts = 6 // 1 initial + up to 5 JS redirects
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			log.Printf("[DetectConfig] Following JS redirect #%d: %s", attempt, currentURL)
		}

		httpReq, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			log.Printf("[DetectConfig] Failed to create request for %s: %v", currentURL, err)
			return ConfigResult{Status: StatusRequestError}
		}
		httpReq.Header.Set("User-Agent", userAgent)
		httpReq.Header.Set("Accept", requestAccept)
		httpReq.Header.Set("Client-ID", state.GetClientID())

		httpResp, err := client.Do(httpReq)
		if err != nil {
			log.Printf("[DetectConfig] HTTP request failed for %s: %v", currentURL, err)
			return ConfigResult{Status: StatusRequestError}
		}
		body, err := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		if err != nil {
			log.Printf("[DetectConfig] Failed to read response body: %v", err)
			return ConfigResult{Status: StatusRequestError}
		}

		resp = httpResp
		content = string(body)

		if verbose {
			log.Printf("[DetectConfig] [Attempt %d] URL: %s", attempt+1, currentURL)
			log.Printf("[DetectConfig] [Attempt %d] Status: %d %s", attempt+1, resp.StatusCode, resp.Status)
			log.Printf("[DetectConfig] [Attempt %d] Final URL: %s", attempt+1, resp.Request.URL.String())
			for k, v := range resp.Header {
				log.Printf("[DetectConfig] [Attempt %d] Header: %s = %s", attempt+1, k, strings.Join(v, ", "))
			}
			log.Printf("[DetectConfig] [Attempt %d] Body length: %d bytes", attempt+1, len(body))
			if len(content) <= 2000 {
				log.Printf("[DetectConfig] [Attempt %d] Body: %s", attempt+1, content)
			} else {
				log.Printf("[DetectConfig] [Attempt %d] Body (first 2000 chars): %s", attempt+1, content[:2000])
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 400 {
			log.Printf("[DetectConfig] Unexpected status code: %d", resp.StatusCode)
			return ConfigResult{Status: StatusRequestError}
		}

		// HTTP 204 = network is truly connected (generate_204 standard response)
		if resp.StatusCode == 204 {
			if verbose {
				log.Println("[DetectConfig] Got HTTP 204 — network is truly connected")
			}
			return ConfigResult{Status: StatusSuccess}
		}

		// Extract CDC headers from this response too
		extractAndSetCDCHeaders(resp, state, verbose)

		// Check for ESurfing portal config tags
		portalConfig = utils.ExtractBetweenTags(content, portalStartTag, portalEndTag)
		if portalConfig != "" {
			if verbose {
				log.Printf("[DetectConfig] Portal config found (%d chars)", len(portalConfig))
				if len(portalConfig) <= 1000 {
					log.Printf("[DetectConfig] Portal config: %s", portalConfig)
				}
			}
			break
		}

		// No config found — try to extract a JavaScript redirect URL
		jsURL := extractJSRedirect(content)
		if jsURL == "" {
			if verbose {
				log.Printf("[DetectConfig] [Attempt %d] No portal config and no JS redirect found", attempt+1)
			}
			break
		}
		currentURL = jsURL
	}

	// After the loop: check if we found portal config
	if portalConfig == "" {
		if len(content) > 0 {
			log.Printf("[DetectConfig] Captive portal detected (HTTP %d with HTML body) but ESurfing portal config not found", resp.StatusCode)
			log.Printf("[DetectConfig] Expected config between tags: %q ... %q", portalStartTag, portalEndTag)
			log.Printf("[DetectConfig] Last URL: %s", currentURL)
			snippet := content
			if len(snippet) > 500 {
				snippet = snippet[:500]
			}
			log.Printf("[DetectConfig] Last page content (up to 500 chars): %s", snippet)
			log.Printf("[DetectConfig] This may be a non-ESurfing portal or the portal page has a different format")
			return ConfigResult{Status: StatusRequestError}
		}
		if verbose {
			log.Println("[DetectConfig] Empty response, network appears connected")
		}
		return ConfigResult{Status: StatusSuccess}
	}

	result := ConfigResult{
		Status:      StatusRequireAuthorization,
		ExtraCfgURL: make(map[string]string),
	}

	result.AuthURL = extractXMLTag(portalConfig, "auth-url")
	result.TicketURL = extractXMLTag(portalConfig, "ticket-url")
	if verbose {
		log.Printf("[DetectConfig] AuthURL: %s", result.AuthURL)
		log.Printf("[DetectConfig] TicketURL: %s", result.TicketURL)
	}

	// Parse extra config URLs from funcfg elements
	parseFuncCfg(portalConfig, result.ExtraCfgURL)
	if verbose {
		for k, v := range result.ExtraCfgURL {
			log.Printf("[DetectConfig] ExtraCfg: %s = %s", k, v)
		}
	}

	if result.AuthURL == "" || result.TicketURL == "" {
		log.Printf("[DetectConfig] Missing AuthURL or TicketURL (auth=%q, ticket=%q)", result.AuthURL, result.TicketURL)
		return ConfigResult{Status: StatusRequestError}
	}

	// Parse userIp and acIp from ticket URL
	parsedURL, err := url.Parse(result.TicketURL)
	if err != nil {
		log.Printf("[DetectConfig] Failed to parse TicketURL: %v", err)
		return ConfigResult{Status: StatusRequestError}
	}

	result.UserIP = parsedURL.Query().Get("wlanuserip")
	result.AcIP = parsedURL.Query().Get("wlanacip")

	if result.UserIP == "" || result.AcIP == "" {
		log.Printf("[DetectConfig] Missing UserIP or AcIP in TicketURL query params")
		return ConfigResult{Status: StatusRequestError}
	}

	if verbose {
		log.Printf("[DetectConfig] UserIP: %s, AcIP: %s", result.UserIP, result.AcIP)
	}

	// Extract area/schoolid/domain — the redirectInterceptor sets them on state
	// during HTTP redirect chain, so read from state as primary source
	if area := state.GetArea(); area != "" {
		result.Area = area
	} else if area := getCDCHeader(resp, "Area"); area != "" {
		result.Area = area
	}
	if schoolID := state.GetSchoolID(); schoolID != "" {
		result.SchoolID = schoolID
	} else if schoolID := getCDCHeader(resp, "SchoolId"); schoolID != "" {
		result.SchoolID = schoolID
	}
	if domain := state.GetDomain(); domain != "" {
		result.Domain = domain
	} else if domain := getCDCHeader(resp, "Domain"); domain != "" {
		result.Domain = domain
	}
	if verbose {
		log.Printf("[DetectConfig] Area: %q, SchoolID: %q, Domain: %q", result.Area, result.SchoolID, result.Domain)
	}

	return result
}

// extractJSRedirect extracts a JavaScript redirect URL from HTML content.
func extractJSRedirect(content string) string {
	patterns := []string{
		`location.href="`,
		`location.replace("`,
		`window.location="`,
		`window.location.href="`,
	}
	for _, pat := range patterns {
		idx := strings.Index(content, pat)
		if idx == -1 {
			continue
		}
		start := idx + len(pat)
		end := strings.Index(content[start:], `"`)
		if end > 0 {
			return content[start : start+end]
		}
	}
	return ""
}

// getCDCHeader extracts a CDC header value, checking both "CDC-Name" and "Name" variants.
func getCDCHeader(resp *http.Response, name string) string {
	if v := resp.Header.Get("CDC-" + name); v != "" {
		return v
	}
	return resp.Header.Get(name)
}

// extractAndSetCDCHeaders extracts CDC routing headers from a response and sets them on state.
func extractAndSetCDCHeaders(resp *http.Response, state StateProvider, verbose bool) {
	if area := getCDCHeader(resp, "Area"); area != "" {
		if verbose {
			log.Printf("[DetectConfig] CDC header Area=%s", area)
		}
		state.SetArea(area)
	}
	if schoolID := getCDCHeader(resp, "SchoolId"); schoolID != "" {
		if verbose {
			log.Printf("[DetectConfig] CDC header SchoolId=%s", schoolID)
		}
		state.SetSchoolID(schoolID)
	}
	if domain := getCDCHeader(resp, "Domain"); domain != "" {
		if verbose {
			log.Printf("[DetectConfig] CDC header Domain=%s", domain)
		}
		state.SetDomain(domain)
	}
}

// extractXMLTag is a simple XML tag extractor (no proper XML parsing needed for these simple tags).
// It also strips CDATA wrappers if present.
func extractXMLTag(xml, tag string) string {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"
	start := strings.Index(xml, startTag)
	if start == -1 {
		return ""
	}
	start += len(startTag)
	end := strings.Index(xml[start:], endTag)
	if end == -1 {
		return ""
	}
	value := xml[start : start+end]
	return stripCDATA(value)
}

// stripCDATA removes <![CDATA[...]]> wrapper if present.
func stripCDATA(s string) string {
	const prefix = "<![CDATA["
	const suffix = "]]>"
	if strings.HasPrefix(s, prefix) && strings.HasSuffix(s, suffix) {
		return s[len(prefix) : len(s)-len(suffix)]
	}
	return s
}

// parseFuncCfg parses funcfg elements from the portal config.
func parseFuncCfg(config string, result map[string]string) {
	// Simple parser for <TagName enable="1" url="..." /> patterns
	pos := 0
	for pos < len(config) {
		funcStart := strings.Index(config[pos:], "<funcfg>")
		if funcStart == -1 {
			break
		}
		funcEnd := strings.Index(config[pos+funcStart:], "</funcfg>")
		if funcEnd == -1 {
			break
		}
		section := config[pos+funcStart : pos+funcStart+funcEnd+len("</funcfg>")]
		// Parse individual elements within funcfg
		parseFuncCfgElements(section, result)
		pos = pos + funcStart + funcEnd + len("</funcfg>")
	}
}

func parseFuncCfgElements(section string, result map[string]string) {
	pos := 0
	for pos < len(section) {
		tagStart := strings.Index(section[pos:], "<")
		if tagStart == -1 {
			break
		}
		tagEnd := strings.Index(section[pos+tagStart:], ">")
		if tagEnd == -1 {
			break
		}
		element := section[pos+tagStart : pos+tagStart+tagEnd+1]
		if strings.Contains(element, "enable=\"1\"") && strings.Contains(element, "url=\"") {
			// Extract tag name
			nameEnd := strings.IndexAny(element[1:], " />")
			if nameEnd > 0 {
				tagName := element[1 : 1+nameEnd]
				// Extract URL
				urlStart := strings.Index(element, "url=\"")
				if urlStart != -1 {
					urlStart += 5
					urlEnd := strings.Index(element[urlStart:], "\"")
					if urlEnd > 0 {
						urlVal := element[urlStart : urlStart+urlEnd]
						result[tagName] = urlVal
					}
				}
			}
		}
		pos = pos + tagStart + tagEnd + 1
	}
}

// CheckVerifyCodeStatus checks if SMS verification is required.
func CheckVerifyCodeStatus(state StateProvider, httpClient *http.Client, username string, extraCfg map[string]string) bool {
	return requestVerifyCode(state, httpClient, username, "QueryVerificateCodeStatus", "11062000", extraCfg)
}

// GetVerifyCode requests a new SMS verification code.
func GetVerifyCode(state StateProvider, httpClient *http.Client, username string, extraCfg map[string]string) bool {
	return requestVerifyCode(state, httpClient, username, "QueryAuthCode", "0", extraCfg)
}

func requestVerifyCode(state StateProvider, httpClient *http.Client, username, reqType, successCode string, extraCfg map[string]string) bool {
	cfgURL, ok := extraCfg[reqType]
	if !ok || cfgURL == "" {
		return false
	}

	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	schoolID := state.GetSchoolID()

	hash := md5.Sum([]byte(schoolID + timestamp + authKey))
	authenticator := strings.ToUpper(hex.EncodeToString(hash[:]))

	reqBody := model.RequireVerificate{
		SchoolID:      schoolID,
		Username:      username,
		Timestamp:     timestamp,
		Authenticator: authenticator,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return false
	}

	req, err := http.NewRequest("POST", cfgURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "okhttp/3.4.1")
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	var result model.ResponseRequireVerificate
	if err := json.Unmarshal(body, &result); err != nil {
		return false
	}

	return result.ResCode == successCode
}
