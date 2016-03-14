package s3git

import (
	"encoding/hex"
	"errors"
	"github.com/s3git/s3git-go/internal/core"
	"github.com/s3git/s3git-go/internal/kv"
	"time"
)

type Commit struct {
	Hash      string
	Message   string
	TimeStamp string
	Parent    string
}

// Perform a commit for the repository
func (repo Repository) Commit(message string) (hash string, empty bool, err error) {
	return repo.commit(message, "master", []string{})
}

// Perform a commit for the named branch of the repository
func (repo Repository) CommitToBranch(message, branch string) (hash string, empty bool, err error) {
	return repo.commit(message, branch, []string{})
}

func (repo Repository) commit(message, branch string, parents []string) (hash string, empty bool, err error) {

	warmParents := []string{}
	coldParents := []string{}

	commits, err := kv.ListTopMostCommits()
	if err != nil {
		return "", false, err
	}

	if len(parents) == 0 {
		for c := range commits {
			warmParents = append(warmParents, hex.EncodeToString(c))
		}
		if len(warmParents) > 1 {
			// TODO: Do extra check whether the trees are the same, in that case we can safely ignore the warning
			return "", false, errors.New("Multiple top most commits founds as parents")
		}
	} else {
		for c := range commits {
			p := hex.EncodeToString(c)
			if contains(parents, p) {
				warmParents = append(warmParents, p)
			} else {
				coldParents = append(coldParents, p)
			}
		}
	}

	return repo.commitWithWarmAndColdParents(message, branch, warmParents, coldParents)
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func (repo Repository) commitWithWarmAndColdParents(message, branch string, warmParents, coldParents []string) (hash string, empty bool, err error) {

	list, err := kv.ListStage()
	if err != nil {
		return "", false, err
	}

	// Create commit object on disk
	commitHash, empty, err := core.StoreCommitObject(message, branch, warmParents, coldParents, list, []string{})
	if err != nil {
		return "", false, err
	}
	if empty {
		return "", true, nil
	}

	// Remove added blobs from staging area
	err = kv.ClearStage()
	if err != nil {
		return "", false, err
	}

	err = core.StorePrefixObject(commitHash)
	if err != nil {
		return "", false, err
	}

	return commitHash, false, nil
}

// List the commits for a repository
func (repo Repository) ListCommits(branch string) (<-chan Commit, error) {

	// TODO: Implement support for branches
	commits, err := kv.ListTopMostCommits()
	if err != nil {
		return nil, err
	}

	inputs := []Commit{}

	for c := range commits {
		commit := hex.EncodeToString(c)
		start, _, err := getCommit(commit)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, *start)
	}

	result := make(chan Commit)

	go func() {
		defer close(result)

		for {
			if len(inputs) == 1 {
				result <- inputs[0]
				input, done, err := getCommit(inputs[0].Parent)
				if err != nil {
					return
				} else if done {
					return // no more new parent --> we are done
				}
				inputs[0] = *input
			} else if len(inputs) == 2 {
				t1, _ := time.Parse(time.RFC3339, inputs[0].TimeStamp)
				t2, _ := time.Parse(time.RFC3339, inputs[1].TimeStamp)

				if inputs[0].Hash == inputs[1].Hash {
					// Same commit object so discard second instance
					pos := 0
					inputs = append(inputs[0:pos], inputs[pos+1:len(inputs)]...)
					result <- inputs[pos]

					input, done, err := getCommit(inputs[pos].Parent)
					if err != nil {
						return
					} else if done {
						return // no more new parent --> we are done
					}
					inputs[pos] = *input

				} else if t1.After(t2) {
					result <- inputs[0]

					input, done, err := getCommit(inputs[0].Parent)
					if err != nil {
						return
					} else if done {
						return // no more new parent --> we are done
					}
					inputs[0] = *input
				} else {
					result <- inputs[1]

					input, done, err := getCommit(inputs[1].Parent)
					if err != nil {
						return
					} else if done {
						return // no more new parent --> we are done
					}
					inputs[1] = *input
				}
			}
		}
	}()

	return result, nil
}

func getCommit(commit string) (*Commit, bool, error) {
	if commit == "" {
		return nil, true, nil // we are done
	}
	co, err := core.GetCommitObject(commit)
	if err != nil {
		return nil, false, err
	}
	result := Commit{Hash: commit, Message: co.S3gitMessage, TimeStamp: co.S3gitTimeStamp}
	if len(co.S3gitWarmParents) == 1 {
		result.Parent = co.S3gitWarmParents[0]
	} else if len(co.S3gitWarmParents) > 1 {
		// TODO: Add other parents to inputs
		result.Parent = co.S3gitWarmParents[0]
	}
	return &result, false, err
}
