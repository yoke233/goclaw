package session

import (
	"testing"
	"time"
)

func TestSessionTreeCreateBranchAndPath(t *testing.T) {
	root := buildSessionWithMessages("root", 2)
	tree, err := NewSessionTree(root)
	if err != nil {
		t.Fatalf("failed to create tree: %v", err)
	}

	branchID, err := tree.CreateBranch("root", nil, "experiment", "user")
	if err != nil {
		t.Fatalf("failed to create branch: %v", err)
	}
	if branchID == "" {
		t.Fatalf("expected non-empty branch ID")
	}

	path, err := tree.GetPath(branchID)
	if err != nil {
		t.Fatalf("failed to get path: %v", err)
	}
	if len(path) != 2 {
		t.Fatalf("expected path length 2, got %d", len(path))
	}
	if path[0].ID != "root" || path[1].ID != branchID {
		t.Fatalf("unexpected path: %s -> %s", path[0].ID, path[1].ID)
	}

	branches := tree.GetBranches()
	if len(branches) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(branches))
	}
}

func TestSessionTreeMaxDepthEnforced(t *testing.T) {
	root := buildSessionWithMessages("root", 1)
	tree, err := NewSessionTree(root)
	if err != nil {
		t.Fatalf("failed to create tree: %v", err)
	}

	tree.SetMaxDepth(1)
	if _, err := tree.CreateBranch("root", nil, "too-deep", "user"); err == nil {
		t.Fatalf("expected depth limit error")
	}
}

func TestSessionTreeMergeBranchTracksMergedFrom(t *testing.T) {
	root := buildSessionWithMessages("root", 1)
	tree, err := NewSessionTree(root)
	if err != nil {
		t.Fatalf("failed to create tree: %v", err)
	}

	branchSession := buildSessionWithMessages("branch-1", 1)
	branchSession.Messages[0].Content = "branch-only"

	if _, err := tree.CreateBranch("root", branchSession, "feature", "user"); err != nil {
		t.Fatalf("failed to create branch: %v", err)
	}

	rootNodeBefore, err := tree.GetNode("root")
	if err != nil {
		t.Fatalf("failed to get root before merge: %v", err)
	}
	beforeCount := len(rootNodeBefore.Session.Messages)

	if err := tree.MergeBranch("branch-1"); err != nil {
		t.Fatalf("failed to merge branch: %v", err)
	}

	rootNodeAfter, err := tree.GetNode("root")
	if err != nil {
		t.Fatalf("failed to get root after merge: %v", err)
	}
	if len(rootNodeAfter.Session.Messages) != beforeCount+1 {
		t.Fatalf("expected root message count %d, got %d", beforeCount+1, len(rootNodeAfter.Session.Messages))
	}

	branchNode, err := tree.GetNode("branch-1")
	if err != nil {
		t.Fatalf("failed to get branch node: %v", err)
	}
	if !branchNode.BranchInfo.IsMerged {
		t.Fatalf("expected branch to be marked as merged")
	}

	found := false
	for _, mergedFrom := range rootNodeAfter.BranchInfo.MergedFrom {
		if mergedFrom == "branch-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected root branch info to include merged branch id")
	}
}

func TestSessionTreeCreateBranchRejectsEmptyBranchID(t *testing.T) {
	root := buildSessionWithMessages("root", 1)
	tree, err := NewSessionTree(root)
	if err != nil {
		t.Fatalf("failed to create tree: %v", err)
	}

	branchSession := buildSessionWithMessages("", 1)
	if _, err := tree.CreateBranch("root", branchSession, "feature", "user"); err == nil {
		t.Fatalf("expected error when branch session key is empty")
	}
}

func TestSessionTreeMergeBranchIsNotAppliedTwice(t *testing.T) {
	root := buildSessionWithMessages("root", 1)
	tree, err := NewSessionTree(root)
	if err != nil {
		t.Fatalf("failed to create tree: %v", err)
	}

	branchSession := buildSessionWithMessages("branch-repeat", 1)
	if _, err := tree.CreateBranch("root", branchSession, "repeat", "user"); err != nil {
		t.Fatalf("failed to create branch: %v", err)
	}

	rootNode, err := tree.GetNode("root")
	if err != nil {
		t.Fatalf("failed to get root node: %v", err)
	}
	before := len(rootNode.Session.Messages)

	if err := tree.MergeBranch("branch-repeat"); err != nil {
		t.Fatalf("first merge should succeed: %v", err)
	}

	afterFirst := len(rootNode.Session.Messages)
	if afterFirst != before+1 {
		t.Fatalf("unexpected message count after first merge: %d", afterFirst)
	}

	if err := tree.MergeBranch("branch-repeat"); err != nil {
		t.Fatalf("second merge should be idempotent or safely ignored, got error: %v", err)
	}

	afterSecond := len(rootNode.Session.Messages)
	if afterSecond != afterFirst {
		t.Fatalf("expected second merge not to duplicate messages, got %d -> %d", afterFirst, afterSecond)
	}
}

func TestSessionTreeMergeDefaultBranchShouldNotDuplicateParentHistory(t *testing.T) {
	root := buildSessionWithMessages("root", 2)
	tree, err := NewSessionTree(root)
	if err != nil {
		t.Fatalf("failed to create tree: %v", err)
	}

	branchID, err := tree.CreateBranch("root", nil, "feature", "user")
	if err != nil {
		t.Fatalf("failed to create default branch: %v", err)
	}

	branchNode, err := tree.GetNode(branchID)
	if err != nil {
		t.Fatalf("failed to get branch node: %v", err)
	}
	branchNode.Session.AddMessage(Message{
		Role:      "assistant",
		Content:   "new-on-branch",
		Timestamp: time.Now(),
	})

	rootNode, err := tree.GetNode("root")
	if err != nil {
		t.Fatalf("failed to get root node: %v", err)
	}
	before := len(rootNode.Session.Messages)

	if err := tree.MergeBranch(branchID); err != nil {
		t.Fatalf("failed to merge branch: %v", err)
	}

	after := len(rootNode.Session.Messages)
	if after != before+1 {
		t.Fatalf("expected merge to append only branch delta (%d -> %d), got %d", before, before+1, after)
	}
}
