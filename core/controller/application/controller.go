// Copyright © 2023 Horizoncd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package application

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/horizoncd/horizon/core/common"
	"github.com/horizoncd/horizon/core/config"
	"github.com/horizoncd/horizon/core/controller/build"
	herrors "github.com/horizoncd/horizon/core/errors"
	"github.com/horizoncd/horizon/lib/q"
	"github.com/horizoncd/horizon/pkg/application/gitrepo"
	applicationmanager "github.com/horizoncd/horizon/pkg/application/manager"
	"github.com/horizoncd/horizon/pkg/application/models"
	applicationservice "github.com/horizoncd/horizon/pkg/application/service"
	applicationregionmanager "github.com/horizoncd/horizon/pkg/applicationregion/manager"
	codemodels "github.com/horizoncd/horizon/pkg/cluster/code"
	clustermanager "github.com/horizoncd/horizon/pkg/cluster/manager"
	"github.com/horizoncd/horizon/pkg/config/template"
	perror "github.com/horizoncd/horizon/pkg/errors"
	eventmodels "github.com/horizoncd/horizon/pkg/event/models"
	eventservice "github.com/horizoncd/horizon/pkg/event/service"
	groupmanager "github.com/horizoncd/horizon/pkg/group/manager"
	groupsvc "github.com/horizoncd/horizon/pkg/group/service"
	membermanager "github.com/horizoncd/horizon/pkg/member/manager"
	membermodels "github.com/horizoncd/horizon/pkg/member/models"
	"github.com/horizoncd/horizon/pkg/param"
	pipelinemanager "github.com/horizoncd/horizon/pkg/pr/pipeline/manager"
	pipelinemodels "github.com/horizoncd/horizon/pkg/pr/pipeline/models"
	regionmodels "github.com/horizoncd/horizon/pkg/region/models"
	tagmanager "github.com/horizoncd/horizon/pkg/tag/manager"
	tagmodels "github.com/horizoncd/horizon/pkg/tag/models"
	trmanager "github.com/horizoncd/horizon/pkg/templaterelease/manager"
	templateschema "github.com/horizoncd/horizon/pkg/templaterelease/schema"
	usersvc "github.com/horizoncd/horizon/pkg/user/service"
	"github.com/horizoncd/horizon/pkg/util/errors"
	"github.com/horizoncd/horizon/pkg/util/jsonschema"
	"github.com/horizoncd/horizon/pkg/util/permission"
	"github.com/horizoncd/horizon/pkg/util/validate"
	"github.com/horizoncd/horizon/pkg/util/wlog"
)

type Controller interface {
	// GetApplication get an application
	GetApplication(ctx context.Context, id uint) (*GetApplicationResponse, error)
	// CreateApplication create an application
	CreateApplication(ctx context.Context, groupID uint,
		request *CreateApplicationRequest) (*GetApplicationResponse, error)
	// UpdateApplication update an application
	UpdateApplication(ctx context.Context, id uint,
		request *UpdateApplicationRequest) (*GetApplicationResponse, error)
	// DeleteApplication delete an application by name
	DeleteApplication(ctx context.Context, id uint, hard bool) error
	// List lists application by query
	List(ctx context.Context, query *q.Query) ([]*ListApplicationResponse, int, error)
	// Transfer  try transfer application to another group
	Transfer(ctx context.Context, id uint, groupID uint) error
	GetSelectableRegionsByEnv(ctx context.Context, id uint, env string) (regionmodels.RegionParts, error)

	CreateApplicationV2(ctx context.Context, groupID uint,
		request *CreateOrUpdateApplicationRequestV2) (*CreateApplicationResponseV2, error)
	UpdateApplicationV2(ctx context.Context, id uint,
		request *CreateOrUpdateApplicationRequestV2) (err error)
	// GetApplicationV2 it can also be used to read v1 repo
	GetApplicationV2(ctx context.Context, id uint) (*GetApplicationResponseV2, error)

	// GetApplicationPipelineStats return pipeline stats about an application
	GetApplicationPipelineStats(ctx context.Context, applicationID uint, cluster string, pageNumber, pageSize int) (
		[]*pipelinemodels.PipelineStats, int64, error)

	Upgrade(ctx context.Context, id uint) error
}

type controller struct {
	applicationGitRepo    gitrepo.ApplicationGitRepo
	templateSchemaGetter  templateschema.Getter
	applicationMgr        applicationmanager.Manager
	applicationSvc        applicationservice.Service
	groupMgr              groupmanager.Manager
	groupSvc              groupsvc.Service
	templateReleaseMgr    trmanager.Manager
	clusterMgr            clustermanager.Manager
	userSvc               usersvc.Service
	memberManager         membermanager.Manager
	eventSvc              eventservice.Service
	tagMgr                tagmanager.Manager
	applicationRegionMgr  applicationregionmanager.Manager
	pipelinemanager       pipelinemanager.Manager
	buildSchema           *build.Schema
	templateUpgradeMapper template.UpgradeMapper
}

var _ Controller = (*controller)(nil)

func NewController(config *config.Config, param *param.Param) Controller {
	return &controller{
		applicationGitRepo:    param.ApplicationGitRepo,
		templateSchemaGetter:  param.TemplateSchemaGetter,
		applicationMgr:        param.ApplicationMgr,
		applicationSvc:        param.ApplicationSvc,
		groupMgr:              param.GroupMgr,
		groupSvc:              param.GroupSvc,
		templateReleaseMgr:    param.TemplateReleaseMgr,
		clusterMgr:            param.ClusterMgr,
		userSvc:               param.UserSvc,
		memberManager:         param.MemberMgr,
		eventSvc:              param.EventSvc,
		tagMgr:                param.TagMgr,
		applicationRegionMgr:  param.ApplicationRegionMgr,
		pipelinemanager:       param.PipelineMgr,
		buildSchema:           param.BuildSchema,
		templateUpgradeMapper: config.TemplateUpgradeMapper,
	}
}

func (c *controller) GetApplication(ctx context.Context, id uint) (_ *GetApplicationResponse, err error) {
	const op = "application controller: get application"
	defer wlog.Start(ctx, op).StopPrint()

	// 1. get application in db
	app, err := c.applicationMgr.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 2. get application jsonBlob in git repo
	applicationRepo, err := c.applicationGitRepo.GetApplication(ctx, app.Name, common.ApplicationRepoDefaultEnv)
	if err != nil {
		return nil, err
	}
	pipelineJSONBlob := applicationRepo.BuildConf
	applicationJSONBlob := applicationRepo.TemplateConf

	// 3. list template releases
	trs, err := c.templateReleaseMgr.ListByTemplateName(ctx, app.Template)
	if err != nil {
		return nil, err
	}

	// 4. get group full path
	group, err := c.groupSvc.GetChildByID(ctx, app.GroupID)
	if err != nil {
		return nil, err
	}
	fullPath := fmt.Sprintf("%v/%v", group.FullPath, app.Name)

	// 5. get tags
	tags, err := c.tagMgr.ListByResourceTypeID(ctx, common.ResourceApplication, app.ID)
	if err != nil {
		return nil, err
	}

	return ofApplicationModel(app, fullPath, trs, pipelineJSONBlob, applicationJSONBlob, tags...), nil
}

func (c *controller) GetApplicationV2(ctx context.Context, id uint) (_ *GetApplicationResponseV2, err error) {
	const op = "application controller: get application v2"
	defer wlog.Start(ctx, op).StopPrint()
	// 1. get application in db
	app, err := c.applicationMgr.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 2. get repo file
	applicationRepo, err := c.applicationGitRepo.GetApplication(ctx, app.Name, common.ApplicationRepoDefaultEnv)
	if err != nil {
		return nil, err
	}

	// 3. get group full path
	group, err := c.groupSvc.GetChildByID(ctx, app.GroupID)
	if err != nil {
		return nil, err
	}
	fullPath := fmt.Sprintf("%v/%v", group.FullPath, app.Name)

	// 4. get tags
	tags, err := c.tagMgr.ListByResourceTypeID(ctx, common.ResourceApplication, app.ID)
	if err != nil {
		return nil, err
	}

	resp := &GetApplicationResponseV2{
		ID:          id,
		Name:        app.Name,
		Description: app.Description,
		Priority:    string(app.Priority),
		Git: func() *codemodels.Git {
			if app.GitURL == "" {
				return nil
			}
			return codemodels.NewGit(app.GitURL, app.GitSubfolder, app.GitRefType, app.GitRef)
		}(),
		Image:       app.Image,
		BuildConfig: applicationRepo.BuildConf,
		Tags:        tagmodels.Tags(tags).IntoTagsBasic(),
		TemplateInfo: func() *codemodels.TemplateInfo {
			if app.Template == "" {
				return nil
			}
			return &codemodels.TemplateInfo{
				Name:    app.Template,
				Release: app.TemplateRelease,
			}
		}(),
		TemplateConfig: applicationRepo.TemplateConf,
		Manifest:       applicationRepo.Manifest,
		FullPath:       fullPath,
		GroupID:        group.ID,
		CreatedAt:      app.CreatedAt,
		UpdatedAt:      app.UpdatedAt,
	}

	return resp, err
}

func (c *controller) CreateApplication(ctx context.Context, groupID uint,
	request *CreateApplicationRequest) (_ *GetApplicationResponse, err error) {
	const op = "application controller: create application"
	defer wlog.Start(ctx, op).StopPrint()

	if err := validateApplicationName(request.Name); err != nil {
		return nil, err
	}
	if err := c.validateCreate(request.Base); err != nil {
		return nil, err
	}
	if request.TemplateInput == nil {
		return nil, err
	}

	if err := c.validateTemplateInput(ctx, request.Template.Name,
		request.Template.Release, request.TemplateInput); err != nil {
		return nil, err
	}
	group, err := c.groupSvc.GetChildByID(ctx, groupID)
	if err != nil {
		return nil, err
	}

	// 2. check groups or applications with the same name exists
	groups, err := c.groupMgr.GetByNameOrPathUnderParent(ctx, request.Name, request.Name, groupID)
	if err != nil {
		return nil, err
	}
	if len(groups) > 0 {
		return nil, perror.Wrap(herrors.ErrNameConflict, "an group with the same name already exists")
	}

	appExistsInDB, err := c.applicationMgr.GetByName(ctx, request.Name)
	if err != nil {
		if _, ok := perror.Cause(err).(*herrors.HorizonErrNotFound); !ok {
			return nil, err
		}
	}
	if appExistsInDB != nil {
		return nil, perror.Wrap(herrors.ErrNameConflict, "an application with the same name already exists, "+
			"please do not create it again")
	}

	// 3. create application in git repo
	createRepoReq := gitrepo.CreateOrUpdateRequest{
		Version:      "",
		Environment:  common.ApplicationRepoDefaultEnv,
		BuildConf:    request.TemplateInput.Pipeline,
		TemplateConf: request.TemplateInput.Application,
	}
	if err := c.applicationGitRepo.CreateOrUpdateApplication(ctx, request.Name, createRepoReq); err != nil {
		return nil, err
	}

	// 4. create application in db
	applicationModel := request.toApplicationModel(groupID)
	applicationModel, err = c.applicationMgr.Create(ctx, applicationModel, request.ExtraMembers)
	if err != nil {
		return nil, err
	}

	// 5. get fullPath
	fullPath := fmt.Sprintf("%v/%v", group.FullPath, applicationModel.Name)

	// 6. list template release
	trs, err := c.templateReleaseMgr.ListByTemplateName(ctx, applicationModel.Template)
	if err != nil {
		return nil, err
	}

	// 7. create tags
	if request.Tags != nil {
		err = c.tagMgr.UpsertByResourceTypeID(ctx, common.ResourceApplication, applicationModel.ID, request.Tags)
		if err != nil {
			return nil, err
		}
	}

	ret := ofApplicationModel(applicationModel, fullPath, trs,
		request.TemplateInput.Pipeline, request.TemplateInput.Application)

	// 7. record event
	c.eventSvc.CreateEventIgnoreError(ctx, common.ResourceApplication, ret.ID,
		eventmodels.ApplicationCreated, nil)
	c.eventSvc.RecordMemberCreatedEvent(ctx, common.ResourceApplication, ret.ID)
	return ret, nil
}

func (c *controller) validateBuildAndTemplateConfigV2(ctx context.Context,
	request *CreateOrUpdateApplicationRequestV2) error {
	if request.TemplateConfig != nil && request.TemplateInfo != nil {
		if err := c.validateTemplateInput(ctx, request.TemplateInfo.Name, request.TemplateInfo.Release, &TemplateInput{
			Application: request.TemplateConfig,
			Pipeline:    nil,
		}); err != nil {
			return err
		}
	}
	if request.BuildConfig != nil {
		if c.buildSchema != nil && c.buildSchema.JSONSchema != nil {
			if err := jsonschema.Validate(c.buildSchema.JSONSchema, request.BuildConfig, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *controller) CreateApplicationV2(ctx context.Context, groupID uint,
	request *CreateOrUpdateApplicationRequestV2) (*CreateApplicationResponseV2, error) {
	const op = "application controller: create application v2"
	defer wlog.Start(ctx, op).StopPrint()

	if err := validateApplicationName(request.Name); err != nil {
		return nil, err
	}
	if request.Priority != nil {
		if err := validatePriority(*request.Priority); err != nil {
			return nil, err
		}
	}
	if request.Git != nil {
		if err := validate.CheckGitURL(request.Git.URL); err != nil {
			return nil, err
		}
	}
	if request.Image != nil {
		if err := validate.CheckImageURL(*request.Image); err != nil {
			return nil, err
		}
	}

	if err := c.validateBuildAndTemplateConfigV2(ctx, request); err != nil {
		return nil, err
	}
	// check groups or applications with the same name exists
	groups, err := c.groupMgr.GetByNameOrPathUnderParent(ctx, request.Name, request.Name, groupID)
	if err != nil {
		return nil, err
	}
	if len(groups) > 0 {
		return nil, perror.Wrap(herrors.ErrNameConflict, "an group with the same name already exists")
	}

	appExistsInDB, err := c.applicationMgr.GetByName(ctx, request.Name)
	if err != nil {
		if _, ok := perror.Cause(err).(*herrors.HorizonErrNotFound); !ok {
			return nil, err
		}
	}
	if appExistsInDB != nil {
		return nil, perror.Wrap(herrors.ErrNameConflict, "an application with the same name already exists, "+
			"please do not create it again")
	}

	// create v2
	createRepoReq := gitrepo.CreateOrUpdateRequest{
		Version:      common.MetaVersion2,
		Environment:  common.ApplicationRepoDefaultEnv,
		BuildConf:    request.BuildConfig,
		TemplateConf: request.TemplateConfig,
	}
	if err := c.applicationGitRepo.CreateOrUpdateApplication(ctx, request.Name, createRepoReq); err != nil {
		return nil, err
	}

	applicationDBModel := request.CreateToApplicationModel(groupID)
	applicationDBModel, err = c.applicationMgr.Create(ctx, applicationDBModel, request.ExtraMembers)
	if err != nil {
		return nil, err
	}

	fullPath, err := func() (string, error) {
		group, err := c.groupSvc.GetChildByID(ctx, groupID)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%v/%v", group.FullPath, request.Name), nil
	}()
	if err != nil {
		return nil, err
	}

	if request.Tags != nil {
		err = c.tagMgr.UpsertByResourceTypeID(ctx, common.ResourceApplication, applicationDBModel.ID, request.Tags)
		if err != nil {
			return nil, err
		}
	}

	ret := &CreateApplicationResponseV2{
		ID:        applicationDBModel.ID,
		Name:      request.Name,
		GroupID:   groupID,
		FullPath:  fullPath,
		CreatedAt: time.Time{},
		UpdatedAt: time.Time{},
	}
	if request.Priority != nil {
		ret.Priority = *request.Priority
	}

	c.eventSvc.CreateEventIgnoreError(ctx, common.ResourceApplication, ret.ID,
		eventmodels.ApplicationCreated, nil)
	c.eventSvc.RecordMemberCreatedEvent(ctx, common.ResourceApplication, ret.ID)
	return ret, nil
}

func (c *controller) UpdateApplication(ctx context.Context, id uint,
	request *UpdateApplicationRequest) (_ *GetApplicationResponse, err error) {
	const op = "application controller: update application"
	defer wlog.Start(ctx, op).StopPrint()

	// 1. get application in db
	appExistsInDB, err := c.applicationMgr.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 2. validate
	if err := c.validateUpdate(request.Base); err != nil {
		return nil, err
	}

	// 3. if templateInput is not empty, validate and update in git repo
	if request.TemplateInput != nil {
		var template, templateRelease string
		if request.Template != nil {
			template = request.Template.Name
			templateRelease = request.Template.Release
		} else {
			template = appExistsInDB.Template
			templateRelease = appExistsInDB.TemplateRelease
		}
		if err := c.validateTemplateInput(ctx, template, templateRelease, request.TemplateInput); err != nil {
			return nil, err
		}

		updateRepoReq := gitrepo.CreateOrUpdateRequest{
			Version:      "",
			Environment:  common.ApplicationRepoDefaultEnv,
			BuildConf:    request.TemplateInput.Pipeline,
			TemplateConf: request.TemplateInput.Application,
		}
		if err := c.applicationGitRepo.CreateOrUpdateApplication(ctx, appExistsInDB.Name, updateRepoReq); err != nil {
			return nil, errors.E(op, err)
		}
	}

	// 4. update application in db
	applicationModel := request.toApplicationModel(appExistsInDB)
	applicationModel, err = c.applicationMgr.UpdateByID(ctx, id, applicationModel)
	if err != nil {
		return nil, err
	}

	// 5. record event
	c.eventSvc.CreateEventIgnoreError(ctx, common.ResourceApplication, applicationModel.ID,
		eventmodels.ApplicationUpdated, nil)

	// 6. get fullPath
	group, err := c.groupSvc.GetChildByID(ctx, appExistsInDB.GroupID)
	if err != nil {
		return nil, err
	}
	fullPath := fmt.Sprintf("%v/%v", group.FullPath, appExistsInDB.Name)

	// 7. list template release
	trs, err := c.templateReleaseMgr.ListByTemplateName(ctx, appExistsInDB.Template)
	if err != nil {
		return nil, err
	}

	// 8. update tags
	if request.Tags != nil {
		err = c.tagMgr.UpsertByResourceTypeID(ctx, common.ResourceApplication, applicationModel.ID, request.Tags)
		if err != nil {
			return nil, err
		}
	}

	return ofApplicationModel(applicationModel, fullPath, trs,
		request.TemplateInput.Pipeline, request.TemplateInput.Application), nil
}

func (c *controller) UpdateApplicationV2(ctx context.Context, id uint,
	request *CreateOrUpdateApplicationRequestV2) (err error) {
	const op = "application controller: update application v2"
	defer wlog.Start(ctx, op).StopPrint()

	// 1. get application in db
	appExistsInDB, err := c.applicationMgr.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if request.Priority != nil {
		if err := validatePriority(*request.Priority); err != nil {
			return err
		}
	}
	if request.Git != nil {
		if err := validate.CheckGitURL(request.Git.URL); err != nil {
			return err
		}
	}
	if request.Image != nil {
		if err := validate.CheckImageURL(*request.Image); err != nil {
			return err
		}
	}

	if err := c.validateBuildAndTemplateConfigV2(ctx, request); err != nil {
		return err
	}
	if (request.TemplateConfig != nil && request.TemplateInfo != nil) || request.BuildConfig != nil {
		updateRepoReq := gitrepo.CreateOrUpdateRequest{
			Version:      common.MetaVersion2,
			Environment:  common.ApplicationRepoDefaultEnv,
			BuildConf:    request.BuildConfig,
			TemplateConf: request.TemplateConfig,
		}
		if err = c.applicationGitRepo.CreateOrUpdateApplication(ctx, appExistsInDB.Name, updateRepoReq); err != nil {
			return err
		}
	}

	// 4. update application in db
	applicationModel := request.UpdateToApplicationModel(appExistsInDB)
	_, err = c.applicationMgr.UpdateByID(ctx, id, applicationModel)
	if err != nil {
		return err
	}

	// 5. update tags
	if request.Tags != nil {
		err = c.tagMgr.UpsertByResourceTypeID(ctx, common.ResourceApplication, id, request.Tags)
		if err != nil {
			return err
		}
	}

	// 6. record event
	c.eventSvc.CreateEventIgnoreError(ctx, common.ResourceApplication, appExistsInDB.ID,
		eventmodels.ApplicationUpdated, nil)
	return err
}

func (c *controller) DeleteApplication(ctx context.Context, id uint, hard bool) (err error) {
	const op = "application controller: delete application"
	defer wlog.Start(ctx, op).StopPrint()

	// 1. get application in db
	app, err := c.applicationMgr.GetByID(ctx, id)
	if err != nil {
		return err
	}

	count, _, err := c.clusterMgr.ListByApplicationID(ctx, id)
	if err != nil {
		return err
	}

	if count > 0 {
		return perror.Wrap(herrors.ErrParamInvalid, "this application cannot be deleted "+
			"because there are clusters under this application.")
	}

	// 2. delete application in git repo
	if hard {
		// delete member
		if err := c.memberManager.HardDeleteMemberByResourceTypeID(ctx,
			string(membermodels.TypeApplication), id); err != nil {
			return err
		}
		// delete region config
		if err := c.applicationRegionMgr.UpsertByApplicationID(ctx, app.ID, nil); err != nil {
			return err
		}
		// delete git repo
		if err := c.applicationGitRepo.HardDeleteApplication(ctx, app.Name); err != nil {
			if _, ok := perror.Cause(err).(*herrors.HorizonErrNotFound); !ok {
				return err
			}
		}
	} else {
		if err := c.applicationGitRepo.HardDeleteApplication(ctx, app.Name); err != nil {
			return err
		}
	}

	// 3. delete application in db
	if err := c.applicationMgr.DeleteByID(ctx, id); err != nil {
		return err
	}

	// 4. record event
	c.eventSvc.CreateEventIgnoreError(ctx, common.ResourceApplication, id,
		eventmodels.ApplicationDeleted, nil)
	return nil
}

func (c *controller) Transfer(ctx context.Context, id uint, groupID uint) error {
	const op = "application controller: transfer application"
	defer wlog.Start(ctx, op).StopPrint()

	group, err := c.groupMgr.GetByID(ctx, groupID)
	if err != nil {
		return err
	}

	return c.applicationMgr.Transfer(ctx, id, group.ID)
}

func (c *controller) validateCreate(b Base) error {
	if err := validatePriority(b.Priority); err != nil {
		return err
	}
	if b.Template == nil {
		return perror.Wrap(herrors.ErrParamInvalid, "template cannot be empty")
	}
	return validateGit(b)
}

func (c *controller) validateUpdate(b Base) error {
	if b.Priority != "" {
		if err := validatePriority(b.Priority); err != nil {
			return err
		}
	}
	return validateGit(b)
}

func validateGit(b Base) error {
	if b.Git != nil && b.Git.URL != "" {
		return validate.CheckGitURL(b.Git.URL)
	}
	return nil
}

// validateTemplateInput validate templateInput is valid for template schema
func (c *controller) validateTemplateInput(ctx context.Context,
	template, release string, templateInput *TemplateInput) error {
	tr, err := c.templateReleaseMgr.GetByTemplateNameAndRelease(ctx, template, release)
	if err != nil {
		return err
	}
	schema, err := c.templateSchemaGetter.GetTemplateSchema(ctx, tr.TemplateName, tr.Name, nil)
	if err != nil {
		return err
	}
	if schema.Application.JSONSchema != nil && templateInput.Application != nil {
		if err := jsonschema.Validate(schema.Application.JSONSchema,
			templateInput.Application, false); err != nil {
			return err
		}
	}
	if schema.Pipeline.JSONSchema != nil && templateInput.Pipeline != nil {
		if err := jsonschema.Validate(schema.Pipeline.JSONSchema, templateInput.Pipeline,
			true); err != nil {
			return err
		}
	}
	return nil
}

// validatePriority validate priority
func validatePriority(priority string) error {
	switch models.Priority(priority) {
	case models.P0, models.P1, models.P2, models.P3:
	default:
		return perror.Wrap(herrors.ErrParamInvalid, "invalid priority")
	}
	return nil
}

// validateApplicationName validate application name
// 1. name length must be less than 40
// 2. name must match pattern ^(([a-z][-a-z0-9]*)?[a-z0-9])?$
func validateApplicationName(name string) error {
	if len(name) == 0 {
		return perror.Wrap(herrors.ErrParamInvalid, "name cannot be empty")
	}

	if len(name) > 40 {
		return perror.Wrap(herrors.ErrParamInvalid, "name must not exceed 40 characters")
	}

	// cannot start with a digit.
	if name[0] >= '0' && name[0] <= '9' {
		return perror.Wrap(herrors.ErrParamInvalid, "name cannot start with a digit")
	}

	pattern := `^(([a-z][-a-z0-9]*)?[a-z0-9])?$`
	r := regexp.MustCompile(pattern)
	if !r.MatchString(name) {
		return perror.Wrap(herrors.ErrParamInvalid,
			fmt.Sprintf("invalid application name, regex used for validation is %v", pattern))
	}

	return nil
}

func (c *controller) List(ctx context.Context, query *q.Query) (
	listApplicationResp []*ListApplicationResponse, count int, err error) {
	const op = "application controller: list application"
	defer wlog.Start(ctx, op).StopPrint()

	subGroupIDs := make([]uint, 0)
	if query.Keywords != nil {
		if userID, ok := query.Keywords[common.ApplicationQueryByUser].(uint); ok {
			if err := permission.OnlySelfAndAdmin(ctx, userID); err != nil {
				return nil, 0, err
			}
			groupIDs, err := c.memberManager.ListResourceOfMemberInfo(ctx, membermodels.TypeGroup, userID)
			if err != nil {
				return nil, 0,
					perror.WithMessage(err, "failed to list group resource of current user")
			}
			subGroups, err := c.groupMgr.GetSubGroupsByGroupIDs(ctx, groupIDs)
			if err != nil {
				return nil, 0, perror.WithMessage(err, "failed to get groups")
			}

			for _, group := range subGroups {
				subGroupIDs = append(subGroupIDs, group.ID)
			}
		} else {
			// TODO: user filter not support group filter yet
			// group filter
			if groupID, ok := query.Keywords[common.ApplicationQueryByGroup].(uint); ok {
				if groupRecursive, ok :=
					query.Keywords[common.ApplicationQueryByGroupRecursive].(bool); ok && groupRecursive {
					groupIDs := []uint{groupID}
					subGroups, err := c.groupMgr.GetSubGroupsByGroupIDs(ctx, groupIDs)
					if err != nil {
						return nil, 0, perror.WithMessage(err, "failed to get groups")
					}
					for _, group := range subGroups {
						subGroupIDs = append(subGroupIDs, group.ID)
					}
				} else {
					subGroupIDs = append(subGroupIDs, groupID)
				}
			}
		}
	}

	listApplicationResp = []*ListApplicationResponse{}
	// 1. get application in db
	count, applications, err := c.applicationMgr.List(ctx, subGroupIDs, query)
	if err != nil {
		return nil, 0, err
	}

	// 2. get groups for full path, full name
	var groupIDs []uint
	for _, application := range applications {
		groupIDs = append(groupIDs, application.GroupID)
	}
	groupMap, err := c.groupSvc.GetChildrenByIDs(ctx, groupIDs)
	if err != nil {
		return nil, count, err
	}

	// 3. convert models.Application to ListApplicationResponse
	for _, application := range applications {
		group, exist := groupMap[application.GroupID]
		if !exist {
			continue
		}
		fullPath := fmt.Sprintf("%v/%v", group.FullPath, application.Name)
		fullName := fmt.Sprintf("%v/%v", group.FullName, application.Name)
		listApplicationResp = append(
			listApplicationResp,
			&ListApplicationResponse{
				FullName:  fullName,
				FullPath:  fullPath,
				Name:      application.Name,
				GroupID:   application.GroupID,
				ID:        application.ID,
				CreatedAt: application.CreatedAt,
				UpdatedAt: application.UpdatedAt,
			},
		)
	}

	return listApplicationResp, count, nil
}

func (c *controller) GetSelectableRegionsByEnv(ctx context.Context, id uint, env string) (
	regionmodels.RegionParts, error) {
	application, err := c.applicationMgr.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	selectableRegionsByEnv, err := c.groupMgr.GetSelectableRegionsByEnv(ctx, application.GroupID, env)
	if err != nil {
		return nil, err
	}

	// set isDefault field
	applicationRegion, err := c.applicationRegionMgr.ListByEnvApplicationID(ctx, env, id)
	if err != nil {
		if _, ok := perror.Cause(err).(*herrors.HorizonErrNotFound); !ok {
			return nil, err
		}
	}
	if applicationRegion != nil {
		for _, regionPart := range selectableRegionsByEnv {
			regionPart.IsDefault = regionPart.Name == applicationRegion.RegionName
		}
	}

	return selectableRegionsByEnv, nil
}

func (c *controller) GetApplicationPipelineStats(ctx context.Context, applicationID uint, cluster string,
	pageNumber, pageSize int) ([]*pipelinemodels.PipelineStats, int64, error) {
	app, err := c.applicationMgr.GetByID(ctx, applicationID)
	if err != nil {
		return nil, 0, err
	}
	if cluster != "" {
		_, err := c.clusterMgr.GetByName(ctx, cluster)
		if err != nil {
			return nil, 0, err
		}
	}

	return c.pipelinemanager.ListPipelineStats(ctx, app.Name, cluster, pageNumber, pageSize)
}

func (c *controller) Upgrade(ctx context.Context, id uint) error {
	const op = "cluster controller: upgrade to v2"
	defer wlog.Start(ctx, op).StopPrint()

	application, err := c.applicationMgr.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// 2. match target template
	targetTemplate, ok := c.templateUpgradeMapper[application.Template]
	if !ok {
		return perror.Wrapf(herrors.ErrParamInvalid,
			"template %s does not support upgrade", application.Template)
	}
	targetRelease, err := c.templateReleaseMgr.GetByTemplateNameAndRelease(ctx,
		targetTemplate.Name, targetTemplate.Release)
	if err != nil {
		return err
	}

	// 3. upgrade git repo files to v2
	err = c.applicationGitRepo.Upgrade(ctx, &gitrepo.UpgradeValuesParam{
		Application:   application.Name,
		TargetRelease: targetRelease,
		BuildConfig:   &targetTemplate.BuildConfig,
	})
	if err != nil {
		return err
	}

	// 4. update template in db
	application.Template = targetRelease.TemplateName
	application.TemplateRelease = targetRelease.Name
	_, err = c.applicationMgr.UpdateByID(ctx, id, application)
	if err != nil {
		return err
	}
	return nil
}
