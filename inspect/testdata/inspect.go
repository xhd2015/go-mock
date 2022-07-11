package testdata

import "context"

func Run(ctx context.Context, status int, _ string) (int, error) {
	return 0, nil
}

func Run2(ctx context.Context, status int, _ string) int { return 0 }

type S struct {
}

func (s *S) Exec(ctx context.Context, status int) string {
	return ""
}
