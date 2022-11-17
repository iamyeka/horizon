package cluster

import (
	"context"
	"fmt"

	"g.hz.netease.com/horizon/core/common"
	herrors "g.hz.netease.com/horizon/core/errors"
	"g.hz.netease.com/horizon/pkg/cluster/cd"
	perror "g.hz.netease.com/horizon/pkg/errors"
	prmodels "g.hz.netease.com/horizon/pkg/pipelinerun/models"
	"g.hz.netease.com/horizon/pkg/util/log"
	"g.hz.netease.com/horizon/pkg/util/wlog"
)

func (c *controller) InternalDeployV2(ctx context.Context, clusterID uint, pipelinerunID uint,
	r interface{}) (_ *InternalDeployResponse, err error) {
	const op = "cluster controller: internal deploy v2"
	defer wlog.Start(ctx, op).StopPrint()

	// 1. get pr, and do some validate
	pr, err := c.pipelinerunMgr.GetByID(ctx, pipelinerunID)
	if err != nil {
		return nil, err
	}
	if pr == nil || pr.ClusterID != clusterID {
		return nil, herrors.NewErrNotFound(herrors.Pipelinerun,
			fmt.Sprintf("cannot find the pipelinerun with id: %v", pipelinerunID))
	}

	// 2. get some relevant models
	cluster, err := c.clusterMgr.GetByID(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	application, err := c.applicationMgr.GetByID(ctx, cluster.ApplicationID)
	if err != nil {
		return nil, err
	}

	// 3. update image in git repo
	tr, err := c.templateReleaseMgr.GetByTemplateNameAndRelease(ctx, cluster.Template, cluster.TemplateRelease)
	if err != nil {
		return nil, err
	}
	log.Infof(ctx, "pipeline %v output content: %v", pipelinerunID, r)
	commit, err := c.clusterGitRepo.UpdatePipelineOutput(ctx, application.Name, cluster.Name,
		tr.ChartName, r)
	if err != nil {
		return nil, perror.WithMessage(err, op)
	}

	// 4. update config commit and status
	if err := c.pipelinerunMgr.UpdateConfigCommitByID(ctx, pr.ID, commit); err != nil {
		return nil, err
	}
	updatePRStatus := func(pState prmodels.PipelineStatus, revision string) error {
		if err = c.pipelinerunMgr.UpdateStatusByID(ctx, pr.ID, pState); err != nil {
			log.Errorf(ctx, "UpdateStatusByID error, pr = %d, status = %s, err = %v",
				pr.ID, pState, err)
			return err
		}
		log.Infof(ctx, "InternalDeploy status, pr = %d, status = %s, revision = %s",
			pr.ID, pState, revision)
		return nil
	}
	if err := updatePRStatus(prmodels.StatusCommitted, commit); err != nil {
		return nil, err
	}

	// 5. merge branch from gitops to master  and update status
	masterRevision, err := c.clusterGitRepo.MergeBranch(ctx, application.Name, cluster.Name, pr.ID)
	if err != nil {
		return nil, perror.WithMessage(err, op)
	}
	if err := updatePRStatus(prmodels.StatusMerged, masterRevision); err != nil {
		return nil, err
	}

	// 6. create cluster in cd system
	regionEntity, err := c.regionMgr.GetRegionEntity(ctx, cluster.RegionName)
	if err != nil {
		return nil, err
	}
	envValue, err := c.clusterGitRepo.GetEnvValue(ctx, application.Name, cluster.Name, tr.ChartName)
	if err != nil {
		return nil, err
	}
	repoInfo := c.clusterGitRepo.GetRepoInfo(ctx, application.Name, cluster.Name)
	if err := c.cd.CreateCluster(ctx, &cd.CreateClusterParams{
		Environment:  cluster.EnvironmentName,
		Cluster:      cluster.Name,
		GitRepoURL:   repoInfo.GitRepoURL,
		ValueFiles:   repoInfo.ValueFiles,
		RegionEntity: regionEntity,
		Namespace:    envValue.Namespace,
	}); err != nil {
		return nil, err
	}

	// 7. reset cluster status
	if cluster.Status == common.ClusterStatusFreed {
		cluster.Status = common.ClusterStatusEmpty
		cluster, err = c.clusterMgr.UpdateByID(ctx, cluster.ID, cluster)
		if err != nil {
			return nil, err
		}
	}

	// 8. deploy cluster in cd system
	if err := c.cd.DeployCluster(ctx, &cd.DeployClusterParams{
		Environment: cluster.EnvironmentName,
		Cluster:     cluster.Name,
		Revision:    masterRevision,
	}); err != nil {
		return nil, err
	}

	// 9. update status
	if err := updatePRStatus(prmodels.StatusOK, masterRevision); err != nil {
		return nil, err
	}

	return &InternalDeployResponse{
		PipelinerunID: pr.ID,
		Commit:        commit,
	}, nil
}