package docker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
	"github.com/bwilczynski/homelab-api/internal/testhelpers"
)

func TestListImages(t *testing.T) {
	resp := testhelpers.LoadFixture[adapters.DSMDockerImageListResponse](t, "testdata/image_list.json")

	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{imagesResp: &resp}})
	result, err := svc.ListImages(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != len(resp.Images) {
		t.Fatalf("expected %d images, got %d", len(resp.Images), len(result.Items))
	}

	first := result.Items[0]
	if first.Id != "nas-01.aabbccddeeff" {
		t.Errorf("expected id nas-01.aabbccddeeff, got %s", first.Id)
	}
	if first.Device != "nas-01" {
		t.Errorf("expected device nas-01, got %s", first.Device)
	}
	if first.Repository != "busybox" {
		t.Errorf("expected repository busybox, got %s", first.Repository)
	}
	if len(first.Tags) != 1 || first.Tags[0] != "latest" {
		t.Errorf("expected tags [latest], got %v", first.Tags)
	}
}

func TestListImagesDeviceFilter(t *testing.T) {
	resp := testhelpers.LoadFixture[adapters.DSMDockerImageListResponse](t, "testdata/image_list.json")
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{imagesResp: &resp}})

	device := "nas-01"
	result, err := svc.ListImages(context.Background(), &device)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != len(resp.Images) {
		t.Fatalf("expected %d images for matching device, got %d", len(resp.Images), len(result.Items))
	}

	other := "nas-02"
	result, err = svc.ListImages(context.Background(), &other)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 images for non-matching device, got %d", len(result.Items))
	}
}

func TestListImagesEmpty(t *testing.T) {
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{
		imagesResp: &adapters.DSMDockerImageListResponse{Images: []adapters.DSMDockerImageItem{}},
	}})
	result, err := svc.ListImages(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Items == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(result.Items))
	}
}

func TestGetImage(t *testing.T) {
	resp := testhelpers.LoadFixture[adapters.DSMDockerImageListResponse](t, "testdata/image_list.json")
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{imagesResp: &resp}})

	detail, err := svc.GetImage(context.Background(), "nas-01.aabbccddeeff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if detail.Id != "nas-01.aabbccddeeff" {
		t.Errorf("expected id nas-01.aabbccddeeff, got %s", detail.Id)
	}
	if detail.Repository != "busybox" {
		t.Errorf("expected repository busybox, got %s", detail.Repository)
	}
	if len(detail.Tags) != 1 || detail.Tags[0] != "latest" {
		t.Errorf("expected tags [latest], got %v", detail.Tags)
	}
	expected := time.Unix(1727386302, 0).UTC()
	if detail.Created != expected {
		t.Errorf("expected created %v, got %v", expected, detail.Created)
	}
	if detail.Size != 4421246 {
		t.Errorf("expected size 4421246, got %d", detail.Size)
	}
	if detail.VirtualSize != 4421246 {
		t.Errorf("expected virtualSize 4421246, got %d", detail.VirtualSize)
	}
	if detail.Description != nil {
		t.Errorf("expected nil description, got %v", detail.Description)
	}
}

func TestGetImageDescriptionOmittedWhenEmpty(t *testing.T) {
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{
		imagesResp: &adapters.DSMDockerImageListResponse{
			Images: []adapters.DSMDockerImageItem{
				{
					ID:          "sha256:aabbccddeeff001122334455667788990011223344556677889900112233445500",
					Repository:  "busybox",
					Tags:        []string{"latest"},
					Size:        4421246,
					VirtualSize: 4421246,
					Created:     1727386302,
					Description: "",
				},
			},
		},
	}})

	detail, err := svc.GetImage(context.Background(), "nas-01.aabbccddeeff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Description != nil {
		t.Errorf("expected nil Description for empty string, got %v", detail.Description)
	}
}

func TestGetImageNotFound(t *testing.T) {
	resp := testhelpers.LoadFixture[adapters.DSMDockerImageListResponse](t, "testdata/image_list.json")
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{imagesResp: &resp}})

	_, err := svc.GetImage(context.Background(), "nas-01.000000000000")
	if err == nil {
		t.Fatal("expected error for unknown image")
	}
	if !errors.Is(err, apierrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetImageInvalidID(t *testing.T) {
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{}})

	_, err := svc.GetImage(context.Background(), "invalid-no-dot")
	if err == nil {
		t.Fatal("expected error for invalid ID")
	}
	if !errors.Is(err, apierrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
