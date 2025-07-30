package main

import (
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"golang.org/x/net/html/charset"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const xmlURL = "https://reports.adviserinfo.sec.gov/reports/CompilationReports/IA_FIRM_SEC_Feed_07_27_2025.xml.gz"
const xmlFile = "sec_adv.xml"

// Firm represents a simplified structure for XML parsing.
type Firm struct {
	XMLName  xml.Name `xml:"Firm"`
	Info     Info     `xml:"Info"`
	MainAddr MainAddr `xml:"MainAddr"`
	Rgstn    Rgstn    `xml:"Rgstn"`
	Filing   Filing   `xml:"Filing"`
	FormInfo FormInfo `xml:"FormInfo"`
}

type Info struct {
	SECRgnCD  string `xml:"SECRgnCD,attr"`
	FirmCrdNb string `xml:"FirmCrdNb,attr"`
	SECNb     string `xml:"SECNb,attr"`
	BusNm     string `xml:"BusNm,attr"`
	LegalNm   string `xml:"LegalNm,attr"`
}

type MainAddr struct {
	Strt1   string `xml:"Strt1,attr"`
	Strt2   string `xml:"Strt2,attr"`
	City    string `xml:"City,attr"`
	State   string `xml:"State,attr"`
	Cntry   string `xml:"Cntry,attr"`
	PostlCd string `xml:"PostlCd,attr"`
	PhNb    string `xml:"PhNb,attr"`
	FaxNb   string `xml:"FaxNb,attr"`
}

type Rgstn struct {
	FirmType string `xml:"FirmType,attr"`
	St       string `xml:"St,attr"`
	Dt       string `xml:"Dt,attr"`
}

type Filing struct {
	Dt       string `xml:"Dt,attr"`
	FormVrsn string `xml:"FormVrsn,attr"`
}

type FormInfo struct {
	Part1A Part1A `xml:"Part1A"`
}

type Part1A struct {
	Item1  Item1  `xml:"Item1"`
	Item3A Item3A `xml:"Item3A"`
	Item5A Item5A `xml:"Item5A"`
	Item5F Item5F `xml:"Item5F"`
	// Add more items if needed for additional fields
}

type Item1 struct {
	Q1F5     string   `xml:"Q1F5,attr"`
	Q1I      string   `xml:"Q1I,attr"`
	Q1M      string   `xml:"Q1M,attr"`
	Q1N      string   `xml:"Q1N,attr"`
	Q1O      string   `xml:"Q1O,attr"`
	Q1P      string   `xml:"Q1P,attr"` // LEI
	WebAddrs WebAddrs `xml:"WebAddrs"`
	DBAList  DBAList  `xml:"DBAList"` // If present
}

type WebAddrs struct {
	WebAddr []string `xml:"WebAddr"`
}

type DBAList struct {
	DBANm []string `xml:"DBANm"`
}

type Item3A struct {
	OrgFormNm    string `xml:"OrgFormNm,attr"`
	OrgFormOthNm string `xml:"OrgFormOthNm,attr"`
}

type Item5A struct {
	TtlEmp string `xml:"TtlEmp,attr"`
}

type Item5F struct {
	Q5F1  string `xml:"Q5F1,attr"`
	Q5F2A string `xml:"Q5F2A,attr"` // Discretionary AUM
	Q5F2B string `xml:"Q5F2B,attr"` // Non-discretionary AUM
	Q5F2C string `xml:"Q5F2C,attr"` // Total AUM
	Q5F2D string `xml:"Q5F2D,attr"`
	Q5F2E string `xml:"Q5F2E,attr"`
	Q5F2F string `xml:"Q5F2F,attr"` // Total clients
	Q5F3  string `xml:"Q5F3,attr"`
}

// DownloadXML downloads the gzipped XML file and decompresses it.
func DownloadXML() error {
	if _, err := os.Stat(xmlFile); err == nil {
		fmt.Println("XML file already exists, skipping download.")
		return nil
	}

	fmt.Println("Downloading and decompressing XML file...")
	resp, err := http.Get(xmlURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download: %s", resp.Status)
	}

	// Create gzip reader
	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer gzipReader.Close()

	// Create output file
	f, err := os.Create(xmlFile)
	if err != nil {
		return err
	}
	defer f.Close()

	// Copy decompressed content
	_, err = io.Copy(f, gzipReader)
	if err != nil {
		return fmt.Errorf("failed to decompress and write file: %v", err)
	}

	fmt.Println("XML file downloaded and decompressed successfully.")
	return nil
}

// ParseAndFilter parses the XML and filters family offices.
func ParseAndFilter() ([]Firm, error) {
	f, err := os.Open(xmlFile)
	if err != nil {
		return nil, err // Fixed: Added comma
	}
	defer f.Close()

	decoder := xml.NewDecoder(f)
	// allow ISO‑8859‑1 (and other HTML‑style encodings) to be
	// converted to UTF‑8
	decoder.CharsetReader = charset.NewReaderLabel
	var inFirms bool
	var firms []Firm

	for {
		t, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err // Fixed: Added comma
		}

		switch se := t.(type) {
		case xml.StartElement:
			if se.Name.Local == "Firms" {
				inFirms = true
			} else if inFirms && se.Name.Local == "Firm" {
				var firm Firm
				if err := decoder.DecodeElement(&firm, &se); err != nil {
					return nil, err
				}
				firms = append(firms, firm)
			}
		case xml.EndElement:
			if se.Name.Local == "Firms" {
				inFirms = false
			}
		}
	}
	return firms, nil
}

// GenerateInsert generates a SQL INSERT statement for a firm.
func GenerateInsert(firm Firm) string {
	// Extract fields
	investorName := escape(firm.Info.BusNm)
	familyOffice := escape(firm.Info.LegalNm)
	crdNumber := escape(firm.Info.FirmCrdNb)
	secNumber := escape(firm.Info.SECNb)
	country := escape(firm.MainAddr.Cntry)
	globalHq := escape(fmt.Sprintf("%s, %s", firm.MainAddr.City, firm.MainAddr.Cntry))
	phone := escape(firm.MainAddr.PhNb)
	dataSource := "SEC"
	verificationStatus := "Unverified"
	registrationStatus := escape(firm.Rgstn.St)
	regDateStr := firm.Rgstn.Dt
	regDate := "NULL"
	if regDateStr != "" {
		regDate = "'" + regDateStr + "'"
	}
	lastUpdatedStr := firm.Filing.Dt
	lastUpdated := "'" + time.Now().Format("2006-01-02 15:04:05") + "'" // Default to now if empty
	if lastUpdatedStr != "" {
		lastUpdated = "'" + lastUpdatedStr + " 00:00:00'"
	}
	lei := escape(firm.FormInfo.Part1A.Item1.Q1P)
	orgForm := escape(firm.FormInfo.Part1A.Item3A.OrgFormNm)
	if firm.FormInfo.Part1A.Item3A.OrgFormOthNm != "" {
		orgForm = escape(firm.FormInfo.Part1A.Item3A.OrgFormOthNm)
	}
	totalEmployees := 0
	if ttl := firm.FormInfo.Part1A.Item5A.TtlEmp; ttl != "" {
		totalEmployees, _ = strconv.Atoi(ttl)
	}
	discretionaryAum := parseDecimal(firm.FormInfo.Part1A.Item5F.Q5F2A)
	nonDiscretionaryAum := parseDecimal(firm.FormInfo.Part1A.Item5F.Q5F2B)
	assetsUnderManagement := parseDecimal(firm.FormInfo.Part1A.Item5F.Q5F2C)
	totalClients := 0
	if tc := firm.FormInfo.Part1A.Item5F.Q5F2F; tc != "" {
		totalClients, _ = strconv.Atoi(tc)
	}
	uniqueKey := escape(firm.Info.FirmCrdNb) // Use CRD as unique
	investorType := escape(firm.Rgstn.FirmType)
	regulatoryFilings := firm.Rgstn.FirmType != "" // Bool true if registered

	// Parse web addrs
	website := ""
	linkedin := ""
	twitter := ""
	for _, wa := range firm.FormInfo.Part1A.Item1.WebAddrs.WebAddr {
		lowerWa := strings.ToLower(wa)
		if strings.Contains(lowerWa, "linkedin.com") {
			linkedin = wa
		} else if strings.Contains(lowerWa, "twitter.com") || strings.Contains(lowerWa, "x.com") {
			twitter = wa
		} else if website == "" {
			website = wa
		}
	}

	// Build INSERT (only key fields populated; others NULL or default)
	columns := "investor_name, family_office, crd_number, sec_number, country, global_hq, phone, data_source, last_updated, verification_status, lei, org_form, number_of_employees, discretionary_aum, non_discretionary_aum, assets_under_management, total_clients, registration_status, registration_date, unique_key, investor_type, regulatory_filings, website, linkedin_profile, twitter_handle"
	values := fmt.Sprintf("'%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', %s, '%s', '%s', '%s', %d, %f, %f, %f, %d, '%s', %s, '%s', '%s', %t, '%s', '%s', '%s'",
		investorName, familyOffice, crdNumber, secNumber, country, globalHq, phone, dataSource, lastUpdated, verificationStatus, lei, orgForm, totalEmployees,
		discretionaryAum, nonDiscretionaryAum, assetsUnderManagement, totalClients, registrationStatus, regDate, uniqueKey, investorType, regulatoryFilings, website, linkedin, twitter)

	return fmt.Sprintf("INSERT INTO public.family_offices (%s) VALUES (%s);", columns, values)
}

// Helper to escape single quotes for SQL.
func escape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// Parse decimal, default 0.
func parseDecimal(s string) float64 {
	if s == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func main() {
	//if err := DownloadXML(); err != nil {
	//	fmt.Printf("Error downloading XML: %v\n", err)
	//	return
	//}

	firms, err := ParseAndFilter()
	if err != nil {
		fmt.Printf("Error parsing XML: %v\n", err)
		return
	}

	fmt.Printf("Found %d potential family offices.\n", len(firms))
	for _, firm := range firms {
		insert := GenerateInsert(firm)
		fmt.Println(insert)
	}
}
