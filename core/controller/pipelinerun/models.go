package pipelinerun

import (
	"time"
)

type GetDiffResponse struct {
	CodeInfo   *CodeInfo   `json:"codeInfo"`
	ConfigDiff *ConfigDiff `json:"configDiff"`
}

type CodeInfo struct {
	// deploy branch info
	Branch string `json:"branch"`
	// branch commit
	CommitID string `json:"commitID"`
	// commit message
	CommitMsg string `json:"commitMsg"`
	// code history link
	Link string `json:"link"`
}

type ConfigDiff struct {
	From string `json:"from"`
	To   string `json:"to"`
	Diff string `json:"diff"`
}

type PipelineBasic struct {
	// ID pipelinerun id
	ID    uint   `json:"id"`
	Title string `json:"title"`
	// Description of this pipelinerun
	Description string `json:"description"`

	// Action type, which can be builddeploy, deploy, restart, rollback
	Action string `json:"action"`
	// Status of this pipelinerun, which can be created, ok, failed, cancelled, unknown
	Status string `json:"status"`
	// Title of this pipelinerun

	// GitURL the git url this pipelinerun to build with, can be empty when action is not builddeploy
	GitURL string `json:"gitURL"`
	// GitBranch the git branch this pipelinerun to build with, can be empty when action is not builddeploy
	GitBranch string `json:"gitBranch"`
	// GitCommit the git commit this pipelinerun to build with, can be empty when action is not builddeploy
	GitCommit string `json:"gitCommit"`
	// ImageURL image url of this pipelinerun to build image
	ImageURL string `json:"imageURL"`

	// LastConfigCommit config commit in master branch of this pipelinerun, can be empty when action is restart
	LastConfigCommit string `json:"lastConfigCommit"`
	// ConfigCommit config commit of this pipelinerun
	ConfigCommit string `json:"configCommit"`

	StartedAt *time.Time `json:"startedAt"`
	// FinishedAt finish time of this pipelinerun
	FinishedAt *time.Time `json:"finishedAt"`
	// createInfo
	CreatedBy UserInfo `json:"createBy"`
}

type UserInfo struct {
	UserID   uint   `json:"userID"`
	UserName string `json:"userName"`
}