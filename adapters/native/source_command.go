package native

import (
	"context"
	"errors"
	"os/exec"
	"strconv"

	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/core/resource"
)

// Lend the original host-owned reader, never resolve its path again. The helper
// owns only its inherited duplicate; the session retains the original resource.
func (a Adapter) sourceCommand(ctx context.Context, r resource.Reader, operation string, args ...string) (*exec.Cmd, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if a.Executable == "" {
		return nil, nil, errors.New("native helper path is empty")
	}
	file, ok := filesystem.BorrowedFile(r)
	if !ok {
		return nil, nil, errors.New("native compressed decoding requires a filesystem-owned resource")
	}
	cmd := exec.CommandContext(ctx, a.Executable)
	configureHelper(cmd)
	handle, release, err := inheritSource(cmd, file)
	if err != nil {
		return nil, nil, err
	}
	cmd.Args = append([]string{a.Executable, operation, strconv.FormatUint(handle, 10)}, args...)
	return cmd, release, nil
}
func (a Adapter) captureSource(ctx context.Context, r resource.Reader, operation string) ([]byte, error) {
	cmd, release, err := a.sourceCommand(ctx, r, operation)
	if err != nil {
		return nil, err
	}
	defer release()
	var output cappedOutput
	output.limit = 4096
	cmd.Stdout = &output
	cmd.Stderr = a.Stderr
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return output.Bytes(), nil
}
