package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/dns"
	"golang.org/x/sync/errgroup"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const (
	cloudFlareZoneIdEnvVar     = "CLOUDFLARE_ZONEID"
	cloudFlareDnsEntryIdEnvVar = "CLOUDFLARE_ENTRY_ID"
)

var (
	client *cloudflare.Client

	ErrEnvVarNotDefined = fmt.Errorf("env variable not defined but required")
)

func main() {
	if printVersion() {
		// do not run further if was asked to print the version
		return
	}
	if err := run(); err != nil {
		log.Fatalf("%s", err)
	}
}

func run() error {
	cfZoneId, cfDnsEntryId, err := input()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var cfIP, currIP net.IP

	errg, gctx := errgroup.WithContext(ctx)
	errg.Go(func() error {
		inCfIP, err := readCloudFlareIP(gctx, cfZoneId, cfDnsEntryId)
		if err != nil {
			return fmt.Errorf("failed reading cloudflare ip: %w", err)
		}
		cfIP = inCfIP
		return nil
	})
	errg.Go(func() error {
		inCurrIP, err := fetchCurrentPublicIP(ctx)
		if err != nil {
			return fmt.Errorf("failed to figure out the current IP assigned by the ISP: %w", err)
		}
		currIP = inCurrIP
		return nil
	})
	if err := errg.Wait(); err != nil {
		return err
	}

	if currIP.Equal(cfIP) {
		log.Printf("no need to update the entry since it contains already the right value. CloudFlare: %s; Current IP: %s\n", cfIP, currIP)
		return nil
	}
	log.Printf("current ip (%s) not the same with the one stored in CloudFlare (%s). updating...\n", currIP.String(), cfIP.String())

	if err := updateCloudFlareIP(ctx, cfZoneId, cfDnsEntryId, currIP.String()); err != nil {
		return fmt.Errorf("failed updating cloudflare ip: %w", err)
	}
	log.Printf("update done from %s to %s", cfIP.String(), currIP.String())
	return nil
}

func fetchCurrentPublicIP(ctx context.Context) (net.IP, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.mapper.ntppool.org/json", nil)
	if err != nil {
		return nil, fmt.Errorf("failed creating request to get current ip: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed requesting the current ip: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	bc, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading the current ip response: %w", err)
	}
	out := struct {
		Http string `json:"HTTP"`
	}{}
	if err := json.Unmarshal(bc, &out); err != nil {
		return nil, fmt.Errorf("failed unmarshalling the current ip response: %w", err)
	}
	return net.ParseIP(out.Http), nil
}

func input() (string, string, error) {
	cfZoneId, ok := os.LookupEnv(cloudFlareZoneIdEnvVar)
	if !ok {
		return "", "", fmt.Errorf("%w: %s", ErrEnvVarNotDefined, cloudFlareZoneIdEnvVar)
	}

	cfDnsEntryId, _ := os.LookupEnv(cloudFlareDnsEntryIdEnvVar)
	if !ok {
		return "", "", fmt.Errorf("%w: %s", ErrEnvVarNotDefined, cloudFlareDnsEntryIdEnvVar)
	}

	return cfZoneId, cfDnsEntryId, nil
}

func cloudFlareClient() *cloudflare.Client {
	if client == nil {
		client = cloudflare.NewClient()
	}
	return client
}

func readCloudFlareIP(ctx context.Context, zoneId, dnsEntryId string) (net.IP, error) {
	cfReadResp, err := cloudFlareClient().DNS.Records.Get(ctx, dnsEntryId, dns.RecordGetParams{
		ZoneID: cloudflare.String(zoneId),
	})
	if err != nil {
		return nil, fmt.Errorf("failed reading the dns entry %q from zone %q: %w", dnsEntryId, zoneId, err)
	}
	return net.ParseIP(cfReadResp.Content), nil
}

func updateCloudFlareIP(ctx context.Context, zoneId, dnsEntryId, dnsEntryContent string) error {
	_, err := cloudFlareClient().DNS.Records.Edit(ctx, dnsEntryId, dns.RecordEditParams{
		ZoneID: cloudflare.String(zoneId),
		Body:   dns.ARecordParam{Content: cloudflare.String(dnsEntryContent)},
	})
	if err != nil {
		return fmt.Errorf("failed updating the dns entry %q from zone %q to %q: %w", dnsEntryId, zoneId, dnsEntryContent, err)
	}
	return nil
}

func printVersion() bool {
	if len(os.Args) == 1 {
		return false
	}
	cmd := strings.TrimSpace(os.Args[1])
	switch cmd {
	case "version":
		fmt.Printf("version: %s\n", version)
		fmt.Printf("commit: %s\n", commit)
		fmt.Printf("date: %s\n", date)
	}
	return true
}
