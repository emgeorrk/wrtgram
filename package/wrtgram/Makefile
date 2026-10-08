#
# Copyright (C) 2026 emgeorrk
#
# This is free software, licensed under the MIT License.
#

include $(TOPDIR)/rules.mk

PKG_NAME:=wrtgram
PKG_VERSION:=0.1.2
PKG_RELEASE:=1

PKG_SOURCE:=$(PKG_NAME)-$(PKG_VERSION).tar.gz
PKG_SOURCE_URL:=https://codeload.github.com/emgeorrk/wrtgram/tar.gz/v$(PKG_VERSION)?
PKG_HASH:=skip

PKG_LICENSE:=MIT
PKG_LICENSE_FILES:=LICENSE
PKG_MAINTAINER:=emgeorrk

PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_BUILD_FLAGS:=no-mips16

GO_PKG:=github.com/emgeorrk/wrtgram
GO_PKG_BUILD_PKG:=github.com/emgeorrk/wrtgram/cmd/wrtgram
GO_PKG_LDFLAGS:=-s -w
GO_PKG_LDFLAGS_X:=main.version=v$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

define Package/wrtgram
  SECTION:=net
  CATEGORY:=Network
  TITLE:=Telegram bot for OpenWrt
  URL:=https://github.com/emgeorrk/wrtgram
  DEPENDS:=$(GO_ARCH_DEPENDS)
endef

define Package/wrtgram/description
  Telegram bot for OpenWrt routers: status, connected devices, VPN tunnels
  with automatic failover, encrypted configuration backups and security
  notifications. Modules enable themselves based on the installed software.
endef

define Package/wrtgram/conffiles
/etc/config/wrtgram
/etc/wrtgram/
endef

define Package/wrtgram/install
	$(call GoPackage/Package/Install/Bin,$(1))
	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/wrtgram.init $(1)/etc/init.d/wrtgram
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/wrtgram.config $(1)/etc/config/wrtgram
	$(INSTALL_DIR) $(1)/etc/uci-defaults
	$(INSTALL_BIN) ./files/wrtgram.defaults $(1)/etc/uci-defaults/99-wrtgram
	$(INSTALL_DIR) $(1)/etc/hotplug.d/dhcp
	$(INSTALL_BIN) ./files/wrtgram.hotplug $(1)/etc/hotplug.d/dhcp/90-wrtgram
endef

$(eval $(call GoBinPackage,wrtgram))
$(eval $(call BuildPackage,wrtgram))
