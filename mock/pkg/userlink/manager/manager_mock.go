// Code generated by MockGen. DO NOT EDIT.
// Source: manager.go

// Package mock_manager is a generated GoMock package.
package mock_manager

import (
	context "context"
	reflect "reflect"

	utils "g.hz.netease.com/horizon/pkg/idp/utils"
	models "g.hz.netease.com/horizon/pkg/userlink/models"
	gomock "github.com/golang/mock/gomock"
)

// MockManager is a mock of Manager interface.
type MockManager struct {
	ctrl     *gomock.Controller
	recorder *MockManagerMockRecorder
}

// MockManagerMockRecorder is the mock recorder for MockManager.
type MockManagerMockRecorder struct {
	mock *MockManager
}

// NewMockManager creates a new mock instance.
func NewMockManager(ctrl *gomock.Controller) *MockManager {
	mock := &MockManager{ctrl: ctrl}
	mock.recorder = &MockManagerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockManager) EXPECT() *MockManagerMockRecorder {
	return m.recorder
}

// CreateLink mocks base method.
func (m *MockManager) CreateLink(ctx context.Context, uid, idpID uint, claims *utils.Claims, deletable bool) (*models.UserLink, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateLink", ctx, uid, idpID, claims, deletable)
	ret0, _ := ret[0].(*models.UserLink)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateLink indicates an expected call of CreateLink.
func (mr *MockManagerMockRecorder) CreateLink(ctx, uid, idpID, claims, deletable interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateLink", reflect.TypeOf((*MockManager)(nil).CreateLink), ctx, uid, idpID, claims, deletable)
}

// DeleteByID mocks base method.
func (m *MockManager) DeleteByID(ctx context.Context, id uint) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteByID", ctx, id)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteByID indicates an expected call of DeleteByID.
func (mr *MockManagerMockRecorder) DeleteByID(ctx, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteByID", reflect.TypeOf((*MockManager)(nil).DeleteByID), ctx, id)
}

// GetByID mocks base method.
func (m *MockManager) GetByID(ctx context.Context, id uint) (*models.UserLink, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetByID", ctx, id)
	ret0, _ := ret[0].(*models.UserLink)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetByID indicates an expected call of GetByID.
func (mr *MockManagerMockRecorder) GetByID(ctx, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetByID", reflect.TypeOf((*MockManager)(nil).GetByID), ctx, id)
}

// GetByIDPAndSub mocks base method.
func (m *MockManager) GetByIDPAndSub(ctx context.Context, idpID uint, sub string) (*models.UserLink, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetByIDPAndSub", ctx, idpID, sub)
	ret0, _ := ret[0].(*models.UserLink)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetByIDPAndSub indicates an expected call of GetByIDPAndSub.
func (mr *MockManagerMockRecorder) GetByIDPAndSub(ctx, idpID, sub interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetByIDPAndSub", reflect.TypeOf((*MockManager)(nil).GetByIDPAndSub), ctx, idpID, sub)
}

// ListByUserID mocks base method.
func (m *MockManager) ListByUserID(ctx context.Context, uid uint) ([]*models.UserLink, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListByUserID", ctx, uid)
	ret0, _ := ret[0].([]*models.UserLink)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListByUserID indicates an expected call of ListByUserID.
func (mr *MockManagerMockRecorder) ListByUserID(ctx, uid interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListByUserID", reflect.TypeOf((*MockManager)(nil).ListByUserID), ctx, uid)
}
