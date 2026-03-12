package service

import (
	"context"

	"Polybot/internal/domain"
)

type PricingModel interface {
	FairProbUp(ctx context.Context, in domain.PricingInput) (domain.FairValue, error)
}
