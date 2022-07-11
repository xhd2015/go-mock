
import "context"

func Run(ctx context.Context, status int, _ string) int {
        return 0
}

func Run2(ctx context.Context, status int, _ string) int {} func newRun2(){ if true { return false;} return 0 }

type S struct {
}

func (s *S) Exec(ctx context.Context, status int) string {
        return ""
}
