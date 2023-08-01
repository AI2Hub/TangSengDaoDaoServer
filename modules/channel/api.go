package channel

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/group"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/user"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/model"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/register"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

type Channel struct {
	ctx *config.Context
	log.Log
	userService      user.IService
	groupService     group.IService
	channelSettingDB *channelSettingDB
}

func New(ctx *config.Context) *Channel {
	return &Channel{
		ctx:              ctx,
		Log:              log.NewTLog("Channel"),
		userService:      user.NewService(ctx),
		groupService:     group.NewService(ctx),
		channelSettingDB: newChannelSettingDB(ctx),
	}
}

// Route 路由配置
func (ch *Channel) Route(r *wkhttp.WKHttp) {
	auth := r.Group("/v1", ch.ctx.AuthMiddleware(r))
	{
		auth.GET("/channel/state", ch.state)
		auth.GET("/channels/:channel_id/:channel_type", ch.channelGet) // 获取频道信息
	}
}

func (ch *Channel) channelGet(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	channelID := c.Param("channel_id")
	channelTypeI64, _ := strconv.ParseInt(c.Param("channel_type"), 10, 64)
	channelType := uint8(channelTypeI64)

	modules := register.GetModules(ch.ctx)
	var err error
	var channelResp *model.ChannelResp
	for _, m := range modules {
		if m.BussDataSource.ChannelGet != nil {
			channelResp, err = m.BussDataSource.ChannelGet(channelID, channelType, loginUID)
			if err != nil {
				ch.Error("查询频道失败！", zap.Error(err))
				c.ResponseError(err)
				return
			}
			break
		}
	}
	if channelResp == nil {
		ch.Error("频道不存在！", zap.String("channel_id", channelID), zap.Uint8("channelType", channelType))
		c.ResponseError(errors.New("频道不存在！"))
		return
	}

	channelSettingM, err := ch.channelSettingDB.queryWithChannel(channelID, channelType) // TODO: 这里虽然暂时看着没啥用，后面可以统一频道的设置信息
	if err != nil {
		ch.Error("查询频道设置失败！", zap.Error(err))
		c.ResponseError(errors.New("查询频道设置失败！"))
		return
	}
	if channelSettingM != nil {
		channelResp.ParentChannel = &struct {
			ChannelID   string `json:"channel_id"`
			ChannelType uint8  `json:"channel_type"`
		}{
			ChannelID:   channelSettingM.ParentChannelID,
			ChannelType: channelSettingM.ParentChannelType,
		}
	}

	c.JSON(http.StatusOK, channelResp)

}

func (ch *Channel) state(c *wkhttp.Context) {
	channelID := c.Query("channel_id")
	channelTypeI64, _ := strconv.ParseInt(c.Query("channel_type"), 10, 64)

	channelType := uint8(channelTypeI64)

	var signalOn uint8 = 0
	var onlineCount int = 0
	if channelType != common.ChannelTypePerson.Uint8() {

		members, err := ch.groupService.GetMembers(channelID)
		if err != nil {
			c.ResponseError(errors.New("查询群成员错误"))
			ch.Error("查询群成员错误", zap.Error(err))
			return
		}
		uids := make([]string, 0)
		if len(members) > 0 {
			for _, member := range members {
				uids = append(uids, member.UID)
			}
		}
		onlineUsers, err := ch.userService.GetUserOnlineStatus(uids)
		if err != nil {
			c.ResponseError(errors.New("查询群成员在线数量错误"))
			ch.Error("查询群成员在线数量错误", zap.Error(err))
			return
		}
		if len(onlineUsers) > 0 {
			for _, user := range onlineUsers {
				if user.Online == 1 {
					onlineCount += 1
				}
			}
		}
	}

	c.Response(stateResp{
		SignalOn:    signalOn,
		OnlineCount: onlineCount,
	})

}

type stateResp struct {
	SignalOn    uint8 `json:"signal_on"`    // 是否可以signal加密聊天
	OnlineCount int   `json:"online_count"` // 成员在线数量
}
