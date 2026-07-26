package toolevents

import "testing"

func TestDetectPrompt(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantHit bool
		wantMsg string
	}{
		// --- Should NOT match ---
		{
			name:    "empty content",
			content: "",
			wantHit: false,
		},
		{
			name:    "whitespace only",
			content: "   \n\n  \n",
			wantHit: false,
		},
		{
			name:    "normal output no prompt",
			content: "Building project...\nCompiling main.go\nDone.\n",
			wantHit: false,
		},
		{
			name:    "long line not a prompt",
			content: "This is a very long line of output that happens to end with a question mark but is really just verbose logging output from some build tool that wraps around? ",
			wantHit: false,
		},
		{
			name:    "shell prompt ❯",
			content: "❯\n\nekristen in 🌐 donnager in tmuxatlas on \ue0a0 copilot [$!?] via 🐹 v1.25.1\n❯ ",
			wantHit: false,
		},
		{
			name:    "shell prompt $",
			content: "user@host:~/projects$ ",
			wantHit: false,
		},
		{
			name:    "shell prompt bare $",
			content: "$ ",
			wantHit: false,
		},
		{
			name:    "shell prompt bare #",
			content: "# ",
			wantHit: false,
		},
		{
			name:    "shell prompt bare %",
			content: "% ",
			wantHit: false,
		},
		{
			name:    "shell prompt with path",
			content: "~/projects/tmuxatlas$ ",
			wantHit: false,
		},
		{
			name:    "codex exited to shell",
			content: "❯ codex\n\nekristen in 🌐 donnager in tmuxatlas on \ue0a0 copilot [$!?] via 🐹 v1.25.1 via 💎 v3.1.3\n❯ ",
			wantHit: false,
		},
		{
			name:    "username containing reject keyword",
			content: "ekristen in 🌐 donnager in tmuxatlas\n❯ ",
			wantHit: false,
		},
		{
			name:    "single key option not enough",
			content: "Something happened\n(y) ",
			wantHit: false,
		},
		{
			name:    "allow in a path or word",
			content: "Processing /var/lib/allow-list/data.json\nDone.\n",
			wantHit: false,
		},

		// --- Should match: yes/no prompts ---
		{
			name:    "y/n approval",
			content: "Do you want to proceed? (y/n) ",
			wantHit: true,
			wantMsg: "Approval prompt detected",
		},
		{
			name:    "Y/n approval",
			content: "Apply changes? [Y/n] ",
			wantHit: true,
			wantMsg: "Approval prompt detected",
		},
		{
			name:    "yes/no prompt",
			content: "Are you sure? yes/no ",
			wantHit: true,
			wantMsg: "Approval prompt detected",
		},

		// --- Should match: parenthesized key options ---
		{
			name:    "codex approve/deny with keys",
			content: "codex wants to run: rm -rf /tmp/test\n(a) Allow  (d) Deny  (e) Explain",
			wantHit: true,
			wantMsg: "Approval prompt detected",
		},
		{
			name:    "multiple key options y/n style",
			content: "Overwrite file?\n(y) Yes  (n) No",
			wantHit: true,
			wantMsg: "Approval prompt detected",
		},

		// --- Should match: numbered options + prompt ---
		{
			name:    "numbered options with prompt",
			content: "Select an option:\n1. Create file\n2. Edit file\n3. Delete file\n> ",
			wantHit: true,
			wantMsg: "Selection prompt detected",
		},

		// --- Should match: input prompts ---
		{
			name:    "question mark prompt",
			content: "What would you like to do? ",
			wantHit: true,
			wantMsg: "Input prompt detected",
		},
		{
			name:    "colon prompt",
			content: "Enter your name: ",
			wantHit: true,
			wantMsg: "Input prompt detected",
		},
		{
			name:    "angle bracket prompt",
			content: "tmuxatlas> ",
			wantHit: true,
			wantMsg: "Input prompt detected",
		},
		{
			name:    "press enter prompt",
			content: "Installation complete.\nPress enter to continue",
			wantHit: true,
			wantMsg: "Input prompt detected",
		},
		{
			name:    "proceed question",
			content: "This will modify 5 files.\nProceed?",
			wantHit: true,
			wantMsg: "Input prompt detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectPrompt(tt.content)
			if result.IsPrompt != tt.wantHit {
				t.Errorf("DetectPrompt() IsPrompt = %v, want %v (got message: %q)", result.IsPrompt, tt.wantHit, result.Message)
			}
			if tt.wantMsg != "" && result.Message != tt.wantMsg {
				t.Errorf("DetectPrompt() Message = %q, want %q", result.Message, tt.wantMsg)
			}
		})
	}
}
