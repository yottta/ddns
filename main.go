package main

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sync/errgroup"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/dns"
)

func main() {
	ctx := context.Background()
	cfZoneId, _ := os.LookupEnv("CLOUDFLARE_ZONEID")
	cfDnsEntryId, _ := os.LookupEnv("CLOUDFLARE_ENTRY_ID")

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
		log.Fatalf("%s", err)
	}

	if currIP.Equal(cfIP) {
		log.Printf("no need to update the entry since it contains already the right value. CloudFlare: %s; Current IP: %s\n", cfIP, currIP)
		return
	}
	fmt.Printf("current ip (%s) not the same with the one stored in CloudFlare (%s). updating...\n", currIP.String(), cfIP.String())

	if err := updateCloudFlareIP(ctx, cfZoneId, cfDnsEntryId, currIP.String()); err != nil {
		log.Fatalf("failed updating cloudflare ip: %s", err)
	}
	fmt.Printf("update done from %s to %s", cfIP.String(), currIP.String())
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

var (
	client *cloudflare.Client
)

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
