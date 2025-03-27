package action

import (
	"errors"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestIsContinue(t *testing.T) {
	tests := []struct {
		name string
		arg  *Result
		want bool
	}{
		{
			name: "continue",
			want: true,
			arg:  nil,
		},
		{
			name: "return",
			want: false,
			arg:  &Result{},
		},
		{
			name: "error",
			want: false,
			arg:  &Result{Err: errors.New("error")},
		},
		{
			name: "terminal error",
			want: false,
			arg:  &Result{Err: reconcile.TerminalError(errors.New("error"))},
		},
		{
			name: "requeue",
			want: false,
			arg:  &Result{Result: reconcile.Result{Requeue: true}},
		},
		{
			name: "requeue after",
			want: false,
			arg:  &Result{Result: reconcile.Result{Requeue: false, RequeueAfter: 5 * time.Second}},
		},
		{
			name: "requeue error",
			want: false,
			arg:  &Result{Result: reconcile.Result{Requeue: true}, Err: errors.New("error")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsContinue(tt.arg); got != tt.want {
				t.Errorf("IsContinue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsError(t *testing.T) {
	tests := []struct {
		name string
		arg  *Result
		want bool
	}{
		{
			name: "continue",
			want: false,
			arg:  nil,
		},
		{
			name: "return",
			want: false,
			arg:  &Result{},
		},
		{
			name: "error",
			want: true,
			arg:  &Result{Err: errors.New("error")},
		},
		{
			name: "terminal error",
			want: true,
			arg:  &Result{Err: reconcile.TerminalError(errors.New("error"))},
		},
		{
			name: "requeue",
			want: false,
			arg:  &Result{Result: reconcile.Result{Requeue: true}},
		},
		{
			name: "requeue after",
			want: false,
			arg:  &Result{Result: reconcile.Result{Requeue: false, RequeueAfter: 5 * time.Second}},
		},
		{
			name: "requeue error",
			want: true,
			arg:  &Result{Result: reconcile.Result{Requeue: true}, Err: errors.New("error")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsError(tt.arg); got != tt.want {
				t.Errorf("IsError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsRequeue(t *testing.T) {
	tests := []struct {
		name string
		arg  *Result
		want bool
	}{
		{
			name: "continue",
			want: false,
			arg:  nil,
		},
		{
			name: "return",
			want: false,
			arg:  &Result{},
		},
		{
			name: "error",
			want: false,
			arg:  &Result{Err: errors.New("error")},
		},
		{
			name: "terminal error",
			want: false,
			arg:  &Result{Err: reconcile.TerminalError(errors.New("error"))},
		},
		{
			name: "requeue",
			want: true,
			arg:  &Result{Result: reconcile.Result{Requeue: true}},
		},
		{
			name: "requeue after",
			want: true,
			arg:  &Result{Result: reconcile.Result{Requeue: false, RequeueAfter: 5 * time.Second}},
		},
		{
			name: "requeue error",
			want: true,
			arg:  &Result{Result: reconcile.Result{Requeue: true}, Err: errors.New("error")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRequeue(tt.arg); got != tt.want {
				t.Errorf("IsRequeue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsReturn(t *testing.T) {
	tests := []struct {
		name string
		arg  *Result
		want bool
	}{
		{
			name: "continue",
			want: false,
			arg:  nil,
		},
		{
			name: "return",
			want: true,
			arg:  &Result{},
		},
		{
			name: "error",
			want: false,
			arg:  &Result{Err: errors.New("error")},
		},
		{
			name: "terminal error",
			want: false,
			arg:  &Result{Err: reconcile.TerminalError(errors.New("error"))},
		},
		{
			name: "requeue",
			want: false,
			arg:  &Result{Result: reconcile.Result{Requeue: true}},
		},
		{
			name: "requeue after",
			want: false,
			arg:  &Result{Result: reconcile.Result{Requeue: false, RequeueAfter: 5 * time.Second}},
		},
		{
			name: "requeue error",
			want: false,
			arg:  &Result{Result: reconcile.Result{Requeue: true}, Err: errors.New("error")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsReturn(tt.arg); got != tt.want {
				t.Errorf("IsReturn() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSuccess(t *testing.T) {
	tests := []struct {
		name string
		arg  *Result
		want bool
	}{
		{
			name: "continue",
			want: true,
			arg:  nil,
		},
		{
			name: "return",
			want: true,
			arg:  &Result{},
		},
		{
			name: "error",
			want: false,
			arg:  &Result{Err: errors.New("error")},
		},
		{
			name: "terminal error",
			want: false,
			arg:  &Result{Err: reconcile.TerminalError(errors.New("error"))},
		},
		{
			name: "requeue",
			want: false,
			arg:  &Result{Result: reconcile.Result{Requeue: true}},
		},
		{
			name: "requeue after",
			want: false,
			arg:  &Result{Result: reconcile.Result{Requeue: false, RequeueAfter: 5 * time.Second}},
		},
		{
			name: "requeue error",
			want: false,
			arg:  &Result{Result: reconcile.Result{Requeue: true}, Err: errors.New("error")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSuccess(tt.arg); got != tt.want {
				t.Errorf("IsSuccess() = %v, want %v", got, tt.want)
			}
		})
	}
}
