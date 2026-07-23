package runtime

import (
	"context"
	"fmt"
	"strings"
)

type MemoryEligibilityService struct {
	store    *Store
	tenantID string
}

func NewMemoryEligibilityService(store *Store, tenantID string) *MemoryEligibilityService {
	return &MemoryEligibilityService{store: store, tenantID: strings.TrimSpace(tenantID)}
}

func (s *MemoryEligibilityService) SetValidity(ctx context.Context, request SetMemoryValidityRequest) (MemoryEligibilityReceipt, error) {
	if err := s.configured(); err != nil {
		return MemoryEligibilityReceipt{}, err
	}
	if err := s.bindTenant(&request.TenantID); err != nil {
		return MemoryEligibilityReceipt{}, err
	}
	return s.store.SetMemoryValidity(ctx, request)
}

func (s *MemoryEligibilityService) Archive(ctx context.Context, request ArchiveMemoryRequest) (MemoryEligibilityReceipt, error) {
	if err := s.configured(); err != nil {
		return MemoryEligibilityReceipt{}, err
	}
	if err := s.bindTenant(&request.TenantID); err != nil {
		return MemoryEligibilityReceipt{}, err
	}
	return s.store.ArchiveMemory(ctx, request)
}

func (s *MemoryEligibilityService) bindTenant(requestTenantID *string) error {
	requested := strings.TrimSpace(*requestTenantID)
	if requested != "" && requested != s.tenantID {
		return fmt.Errorf("request tenant_id does not match configured tenant")
	}
	*requestTenantID = s.tenantID
	return nil
}

func (s *MemoryEligibilityService) configured() error {
	if s == nil || s.store == nil {
		return fmt.Errorf("memory eligibility store is not configured")
	}
	if s.tenantID == "" {
		return fmt.Errorf("memory eligibility tenant is not configured")
	}
	return nil
}
