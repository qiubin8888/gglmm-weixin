package account

import (
	"encoding/json"
	"net/http"

	"github.com/weihongguo/gglmm"
)

// WechatMiniProgramUserInfoService 微信登录服务
type WechatMiniProgramUserInfoService struct {
	repository *gglmm.GormRepository
	jwtExpires int64
	jwtSecret  string
}

// NewWechatMiniProgramUserInfoService 新建用户服务
func NewWechatMiniProgramUserInfoService(jwtExpires int64, jwtSecret string) *WechatMiniProgramUserInfoService {
	return &WechatMiniProgramUserInfoService{
		repository: gglmm.DefaultGormRepository(),
		jwtExpires: jwtExpires,
		jwtSecret:  jwtSecret,
	}
}

// NewWechatMiniProgramUserInfoServiceConfig 新建用户服务
func NewWechatMiniProgramUserInfoServiceConfig(config ConfigJWT) *WechatMiniProgramUserInfoService {
	return NewWechatMiniProgramUserInfoService(config.Expires, config.Secret)
}

// CustomActions --
func (service *WechatMiniProgramUserInfoService) CustomActions() ([]*gglmm.HTTPAction, error) {
	actions := []*gglmm.HTTPAction{
		gglmm.NewHTTPAction("/wechat-mini-program/user-info", service.MiniProgramUserInfo, "PUT"),
	}
	return actions, nil
}

// RESTAction --
func (service *WechatMiniProgramUserInfoService) RESTAction(action gglmm.RESTAction) (*gglmm.HTTPAction, error) {
	return nil, nil
}

// MiniProgramUserInfo --
func (service *WechatMiniProgramUserInfoService) MiniProgramUserInfo(w http.ResponseWriter, r *http.Request) {
	userID, err := GetAuthID(r, AuthTypeUser)
	if err != nil {
		gglmm.NewFailResponse("claims subject").WriteJSON(w)
		return
	}

	decoder := json.NewDecoder(r.Body)
	userInfoRequest := WechatMiniProgramUserInfoRequest{}
	if err := decoder.Decode(&userInfoRequest); err != nil {
		gglmm.NewFailResponse(err.Error()).WriteJSON(w)
		return
	}

	if userInfoRequest.Check("raw") {
		service.miniProgramRawUserInfo(w, userID, userInfoRequest)
		return
	}

	if userInfoRequest.Check("encrypted") {
		service.miniProgramEncryptedUserInfo(w, userID, userInfoRequest)
		return
	}

	gglmm.NewFailResponse("check fail").WriteJSON(w)
}

// rawUserInfo 更新用户信息
// 下发新的authInfo
func (service *WechatMiniProgramUserInfoService) miniProgramRawUserInfo(w http.ResponseWriter, userID int64, userInfoRequest WechatMiniProgramUserInfoRequest) {
	wechatUser := WechatMiniProgramUser{}
	filterRequest := gglmm.FilterRequest{}
	filterRequest.AddFilter("user_id", gglmm.FilterOperateEqual, userID)
	if err := service.repository.Get(&wechatUser, filterRequest); err != nil {
		gglmm.NewFailResponse(err.Error()).WriteJSON(w)
		return
	}
	if !userInfoRequest.CheckSignature(wechatUser.SessionKey) {
		gglmm.NewFailResponse("signature chect invalid").WriteJSON(w)
		return
	}

	user := User{}
	idRequest := gglmm.IDRequest{
		ID:       userID,
		Preloads: []string{"UserInfo"},
	}
	if err := service.repository.Get(&user, idRequest); err != nil {
		gglmm.NewFailResponse(err.Error()).WriteJSON(w)
		return
	}
	if err := service.miniProgramUpdateUser(&wechatUser, &user, &userInfoRequest.UserInfo); err != nil {
		gglmm.NewFailResponse(err.Error()).WriteJSON(w)
		return
	}
	gglmm.NewSuccessResponse().
		AddData("authInfo", user.AuthInfo()).
		WriteJSON(w)
}

// encryptedUserInfo 解析加密数据，更新用户信息
// 下发新的authToken
// 下发新的authInfo
func (service *WechatMiniProgramUserInfoService) miniProgramEncryptedUserInfo(w http.ResponseWriter, userID int64, userInfoRequest WechatMiniProgramUserInfoRequest) {
	wechatUser := WechatMiniProgramUser{}
	filterRequest := gglmm.FilterRequest{}
	filterRequest.AddFilter("user_id", gglmm.FilterOperateEqual, userID)
	if err := service.repository.Get(&wechatUser, filterRequest); err != nil {
		gglmm.NewFailResponse(err.Error()).WriteJSON(w)
		return
	}
	wechatUserInfo, err := userInfoRequest.Decrypt(wechatUser.SessionKey)
	if err != nil {
		gglmm.NewFailResponse(err.Error()).WriteJSON(w)
		return
	}
	user := User{}
	idRequest := gglmm.IDRequest{
		ID:       userID,
		Preloads: []string{"UserInfo"},
	}
	if err = service.repository.Get(&user, idRequest); err != nil {
		gglmm.NewFailResponse(err.Error()).WriteJSON(w)
		return
	}
	if err = service.miniProgramUpdateUser(&wechatUser, &user, wechatUserInfo); err != nil {
		gglmm.NewFailResponse(err.Error()).WriteJSON(w)
		return
	}
	authToken, jwtClaims, err := GenerateAuthToken(user, service.jwtExpires, service.jwtSecret)
	if err != nil {
		gglmm.NewFailResponse(err.Error()).WriteJSON(w)
		return
	}
	gglmm.NewSuccessResponse().
		AddData("authToken", authToken).
		AddData("authIssuedAt", jwtClaims.IssuedAt).
		AddData("authExpiresAt", jwtClaims.ExpiresAt).
		AddData("authInfo", user.AuthInfo()).
		WriteJSON(w)
}

func (service *WechatMiniProgramUserInfoService) miniProgramUpdateUser(miniProgramUser *WechatMiniProgramUser, user *User, userInfo *WechatMiniProgramUserInfo) error {
	tx := service.repository.Begin()

	miniProgramUserUpdates := map[string]interface{}{
		"nickname":   userInfo.Nickname,
		"avatar_url": userInfo.AvatarURL,
		"gender":     userInfo.Gender,
		"province":   userInfo.Province,
		"city":       userInfo.City,
		"country":    userInfo.Country,
		"language":   userInfo.Language,
	}
	if err := tx.Model(miniProgramUser).Updates(miniProgramUserUpdates).Error; err != nil {
		tx.Rollback()
		return err
	}

	userInfoUpdates := map[string]interface{}{
		"nickname":   userInfo.Nickname,
		"avatar_url": userInfo.AvatarURL,
	}
	if err := tx.Model(user.UserInfo).Updates(userInfoUpdates).Error; err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()
	return nil
}
