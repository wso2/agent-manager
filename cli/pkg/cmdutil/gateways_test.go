// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// Licensed under the Apache License, Version 2.0.
package cmdutil

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
)

// pagedGatewayServer serves `total` synthetic gateways, honouring the limit and
// offset the client asks for, and records each offset it was called with.
func pagedGatewayServer(t *testing.T, total int) (*aigateway.ClientWithResponses, *[]string) {
	t.Helper()
	var offsets []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		offsets = append(offsets, query.Get("offset"))
		offset, _ := strconv.Atoi(query.Get("offset"))
		limit, _ := strconv.Atoi(query.Get("limit"))

		page := []aigateway.GatewayDocument{}
		for i := offset; i < offset+limit && i < total; i++ {
			page = append(page, aigateway.GatewayDocument{
				Metadata: aigateway.DocumentMetadata{Name: "gw-" + strconv.Itoa(i)},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		n, all, l, o := int32(len(page)), int32(total), int32(limit), int32(offset)
		_ = json.NewEncoder(w).Encode(aigateway.GatewayListResponse{
			Count:      &n,
			List:       page,
			Pagination: &aigateway.Pagination{Total: &all, Limit: &l, Offset: &o},
		})
	}))
	t.Cleanup(srv.Close)

	client, err := aigateway.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatalf("NewClientWithResponses: %v", err)
	}
	return client, &offsets
}

// A single unpaginated request stopped at the server's page size, so a gateway past
// that boundary was invisible to both name resolution and completion.
func TestListAllGateways_FollowsEveryPage(t *testing.T) {
	const total = 250 // more than two pages at gatewayPageLimit
	client, offsets := pagedGatewayServer(t, total)

	got, err := ListAllGateways(context.Background(), client, "acme")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != total {
		t.Fatalf("len = %d, want %d", len(got), total)
	}
	if got[0].Metadata.Name != "gw-0" || got[total-1].Metadata.Name != "gw-"+strconv.Itoa(total-1) {
		t.Errorf("bounds = %q..%q, want gw-0..gw-%d", got[0].Metadata.Name, got[total-1].Metadata.Name, total-1)
	}
	want := []string{"0", "100", "200"}
	if len(*offsets) != len(want) {
		t.Fatalf("offsets = %v, want %v", *offsets, want)
	}
	for i := range want {
		if (*offsets)[i] != want[i] {
			t.Errorf("offsets[%d] = %q, want %q", i, (*offsets)[i], want[i])
		}
	}
}

func TestListAllGateways_SinglePageMakesOneRequest(t *testing.T) {
	client, offsets := pagedGatewayServer(t, 3)

	got, err := ListAllGateways(context.Background(), client, "acme")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("len = %d, want 3", len(got))
	}
	if len(*offsets) != 1 {
		t.Errorf("made %d requests, want 1", len(*offsets))
	}
}

func TestListAllGateways_EmptyOrgMakesOneRequest(t *testing.T) {
	client, offsets := pagedGatewayServer(t, 0)

	got, err := ListAllGateways(context.Background(), client, "acme")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
	if len(*offsets) != 1 {
		t.Errorf("made %d requests, want 1", len(*offsets))
	}
}

// A Total the server over-reports must not spin the loop forever: an empty page
// terminates on its own.
func TestListAllGateways_StopsOnEmptyPageDespiteOverstatedTotal(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		total := int32(99)
		_ = json.NewEncoder(w).Encode(aigateway.GatewayListResponse{
			List:       []aigateway.GatewayDocument{},
			Pagination: &aigateway.Pagination{Total: &total},
		})
	}))
	defer srv.Close()

	client, err := aigateway.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatalf("NewClientWithResponses: %v", err)
	}

	got, err := ListAllGateways(context.Background(), client, "acme")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
	if calls != 1 {
		t.Errorf("made %d requests, want 1", calls)
	}
}

// Shell completion runs under a 2s timeout that paging a large org can outrun. The
// pages already gathered are returned alongside the error so a best-effort caller can
// offer a truncated list instead of nothing.
func TestListAllGateways_ReturnsPagesGatheredBeforeAFailure(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		page := make([]aigateway.GatewayDocument, 0, int(gatewayPageLimit))
		for i := range int(gatewayPageLimit) {
			page = append(page, aigateway.GatewayDocument{Metadata: aigateway.DocumentMetadata{Name: "gw-" + strconv.Itoa(i)}})
		}
		w.Header().Set("Content-Type", "application/json")
		total := gatewayPageLimit * 2
		_ = json.NewEncoder(w).Encode(aigateway.GatewayListResponse{
			List:       page,
			Pagination: &aigateway.Pagination{Total: &total},
		})
	}))
	defer srv.Close()

	client, err := aigateway.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatalf("NewClientWithResponses: %v", err)
	}

	got, err := ListAllGateways(context.Background(), client, "acme")
	if err == nil {
		t.Fatal("expected an error from the second page")
	}
	if len(got) != int(gatewayPageLimit) {
		t.Errorf("len = %d, want the %d gateways from the first page", len(got), gatewayPageLimit)
	}
}

func TestListAllGateways_ServerErrorIsSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "INTERNAL_ERROR", "message": "boom"})
	}))
	defer srv.Close()

	client, err := aigateway.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatalf("NewClientWithResponses: %v", err)
	}

	if _, err := ListAllGateways(context.Background(), client, "acme"); err == nil {
		t.Fatal("expected an error on 500")
	}
}
