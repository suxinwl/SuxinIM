/*
 Navicat Premium Dump SQL

 Source Server         : 本地数据库
 Source Server Type    : MySQL
 Source Server Version : 80012 (8.0.12)
 Source Host           : localhost:3306
 Source Schema         : goframepro

 Target Server Type    : MySQL
 Target Server Version : 80012 (8.0.12)
 File Encoding         : 65001

 Date: 28/04/2026 16:54:47
*/

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- ----------------------------
-- Table structure for gf_admin
-- ----------------------------
DROP TABLE IF EXISTS `gf_admin`;
CREATE TABLE `gf_admin`  (
  `id` int(10) UNSIGNED NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `account_id` int(11) NOT NULL DEFAULT 0 COMMENT '账号id/记录那个账号添加',
  `dept_id` int(11) NOT NULL DEFAULT 0 COMMENT '部门id',
  `username` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT NULL COMMENT '用户名',
  `password` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT NULL COMMENT '密码',
  `salt` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT NULL COMMENT '密码盐',
  `name` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT NULL COMMENT '姓名',
  `nickname` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT NULL COMMENT '昵称',
  `avatar` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT NULL COMMENT '头像',
  `email` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT NULL COMMENT '电子邮箱',
  `mobile` varchar(11) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT NULL COMMENT '手机号码',
  `tel` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '备用电话用户自己填写',
  `status` tinyint(1) NOT NULL DEFAULT 0 COMMENT '状态:0=正常,1=禁用',
  `remark` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '备注',
  `loginip` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT NULL COMMENT '登录IP',
  `logintime` datetime NULL DEFAULT NULL COMMENT '最后登录时间',
  `login_attempts` tinyint(1) NOT NULL DEFAULT 0 COMMENT '登录尝试次数',
  `lock_time` datetime NULL DEFAULT NULL COMMENT '账号锁定时间',
  `createtime` datetime NULL DEFAULT NULL COMMENT '创建时间',
  `updatetime` datetime NULL DEFAULT NULL COMMENT '更新时间',
  `deletetime` datetime NULL DEFAULT NULL COMMENT '删除时间',
  `pwd_reset_time` datetime NULL DEFAULT NULL COMMENT '修改密码时间',
  PRIMARY KEY (`id`) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 4 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_general_ci COMMENT = '管理员表' ROW_FORMAT = DYNAMIC;

-- ----------------------------
-- Records of gf_admin
-- ----------------------------

-- ----------------------------
-- Table structure for gf_attachment
-- ----------------------------
DROP TABLE IF EXISTS `gf_attachment`;
CREATE TABLE `gf_attachment`  (
  `id` int(10) UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '文件类型0=图片，1=文件夹,2=视频，3=音频,4=文档',
  `business_id` int(11) NOT NULL DEFAULT 0 COMMENT '业务主账号id',
  `pid` int(11) NOT NULL DEFAULT 0 COMMENT '附件',
  `location` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'local' COMMENT '图片存储位置:local=本地,alioss=阿里云,tencentcos=腾讯云,qiniuoss=七牛云',
  `name` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '附件原来名称',
  `title` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '文件名称',
  `type` tinyint(1) NOT NULL DEFAULT 0 COMMENT '文件类型0=图片，1=文件夹,2=视频，3=音频,4=文档,5=其他',
  `weigh` int(11) NOT NULL DEFAULT 0 COMMENT '排序',
  `url` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '访问路径',
  `imagewidth` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '宽度',
  `imageheight` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '高度',
  `filesize` bigint UNSIGNED NOT NULL DEFAULT 0 COMMENT '文件大小',
  `mimetype` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'mime类型',
  `extparam` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '透传数据',
  `storage` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'local' COMMENT '存储位置',
  `cover_url` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '视频封面',
  `sha1` varchar(40) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '文件 sha1编码',
  `is_common` tinyint(1) NOT NULL DEFAULT 0 COMMENT '是否公共1=是',
  `createtime` datetime NULL DEFAULT NULL COMMENT '上传时间',
  PRIMARY KEY (`id`) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 74 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_general_ci COMMENT = '客户端附件' ROW_FORMAT = DYNAMIC;

-- ----------------------------
-- Records of gf_attachment
-- ----------------------------

-- ----------------------------
-- Table structure for gf_auth_dept
-- ----------------------------
DROP TABLE IF EXISTS `gf_auth_dept`;
CREATE TABLE `gf_auth_dept`  (
  `id` int(10) UNSIGNED NOT NULL AUTO_INCREMENT,
  `business_id` int(11) NOT NULL DEFAULT 1 COMMENT '业务主账号id',
  `account_id` int(11) NOT NULL DEFAULT 0 COMMENT '添加账号',
  `name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '部门名称',
  `pid` int(11) NOT NULL DEFAULT 0 COMMENT '上级部门',
  `weigh` int(11) NOT NULL DEFAULT 0 COMMENT '排序',
  `status` tinyint(1) NOT NULL DEFAULT 0 COMMENT '状态:0=正常;1=禁用',
  `im_group_enabled` tinyint(1) NOT NULL DEFAULT 0 COMMENT '部门群自动管理开关',
  `leader_uid` bigint NOT NULL DEFAULT 0 COMMENT '部门负责人宿主UID',
  `remark` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '备注',
  `createtime` datetime NULL DEFAULT NULL COMMENT '创建时间',
  `updatetime` datetime NULL DEFAULT NULL COMMENT '更新时间',
  `deletetime` datetime NULL DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 15 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_general_ci COMMENT = '管理后台部门' ROW_FORMAT = DYNAMIC;

-- ----------------------------
-- Records of gf_auth_dept
-- ----------------------------

-- ----------------------------
-- Table structure for gf_auth_role
-- ----------------------------
DROP TABLE IF EXISTS `gf_auth_role`;
CREATE TABLE `gf_auth_role`  (
  `id` int(10) UNSIGNED NOT NULL AUTO_INCREMENT,
  `business_id` int(11) NOT NULL DEFAULT 0 COMMENT '业务主账号id',
  `account_id` int(11) NOT NULL DEFAULT 0 COMMENT '添加用户id',
  `pid` int(11) NOT NULL DEFAULT 0 COMMENT '父级',
  `name` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '名称',
  `rules` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '规则ID 所拥有的权限包括父级',
  `menu` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '选择的id，用于编辑赋值',
  `btns` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '按钮id，用于编辑赋值',
  `status` tinyint(1) NOT NULL DEFAULT 0 COMMENT '状态1=禁用',
  `data_access` tinyint(1) NOT NULL DEFAULT 0 COMMENT '数据权限0=自己1=自己及子权限，2=全部',
  `remark` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '描述',
  `weigh` int(11) NOT NULL COMMENT '排序',
  `createtime` datetime NULL DEFAULT NULL COMMENT '添加时间',
  PRIMARY KEY (`id`) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 36 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_general_ci COMMENT = '权限分组' ROW_FORMAT = DYNAMIC;

-- ----------------------------
-- Records of gf_auth_role
-- ----------------------------
INSERT INTO `gf_auth_role` VALUES (1, 0, 0, 0, '超级管理组', '*', '*', '', 0, 2, '超级管理权限-系统其他权限都继承它', 1, '2024-02-05 15:35:59');

-- ----------------------------
-- Table structure for gf_auth_role_access
-- ----------------------------
DROP TABLE IF EXISTS `gf_auth_role_access`;
CREATE TABLE `gf_auth_role_access`  (
  `uid` int(11) NOT NULL DEFAULT 0 COMMENT '账号id',
  `role_id` int(11) NOT NULL DEFAULT 0 COMMENT '授权id'
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_general_ci COMMENT = '商务端菜单授权' ROW_FORMAT = DYNAMIC;

-- ----------------------------
-- Records of gf_auth_role_access
-- ----------------------------
INSERT INTO `gf_auth_role_access` VALUES (1, 1);

-- ----------------------------
-- Table structure for gf_auth_rule
-- ----------------------------
DROP TABLE IF EXISTS `gf_auth_rule`;
CREATE TABLE `gf_auth_rule`  (
  `id` int(10) UNSIGNED NOT NULL AUTO_INCREMENT,
  `uid` int(11) NOT NULL DEFAULT 0 COMMENT '添加用户',
  `title` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '菜单名称',
  `des` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '描述',
  `locale` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '中英文标题key',
  `weigh` int(11) NOT NULL DEFAULT 0 COMMENT '排序',
  `type` tinyint(1) NOT NULL DEFAULT 0 COMMENT '类型 0=目录，1=菜单，2=按钮',
  `pid` int(11) NOT NULL DEFAULT 0 COMMENT '上一级',
  `icon` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '图标',
  `routepath` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '路由地址',
  `routename` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '路由名称',
  `component` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '组件路径',
  `redirect` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '重定向地址',
  `path` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '接口路径',
  `permission` varchar(80) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT NULL COMMENT '权限标识',
  `status` tinyint(1) NOT NULL DEFAULT 0 COMMENT '状态 0=启用1=禁用',
  `isext` tinyint(1) NOT NULL DEFAULT 0 COMMENT '是否外链 0=否1=是',
  `keepalive` tinyint(1) NOT NULL DEFAULT 0 COMMENT '是否缓存 0=否1=是',
  `requiresauth` tinyint(1) NOT NULL DEFAULT 1 COMMENT '是否需要登录鉴权 0=否1=是',
  `hideinmenu` tinyint(1) NOT NULL DEFAULT 0 COMMENT '是否在左侧菜单中隐藏该项 0=否1=是',
  `hidechildreninmenu` tinyint(1) NOT NULL DEFAULT 0 COMMENT '强制在左侧菜单中显示单项 0=否1=是',
  `activemenu` tinyint(1) NOT NULL DEFAULT 0 COMMENT '高亮设置的菜单项 0=否1=是',
  `noaffix` tinyint(1) NOT NULL DEFAULT 0 COMMENT '如果设置为true，标签将不会添加到tab-bar中 0=否1=是',
  `onlypage` tinyint(1) NOT NULL DEFAULT 0 COMMENT '独立页面不需layout和登录，如登录页、数据大屏',
  `createtime` datetime NULL DEFAULT NULL COMMENT '创建时间',
  `updatetime` datetime NULL DEFAULT NULL COMMENT '更新时间',
  `deletetime` datetime NULL DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 83 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_general_ci COMMENT = 'B端后台菜单' ROW_FORMAT = DYNAMIC;

-- ----------------------------
-- Records of gf_auth_rule
-- ----------------------------
INSERT INTO `gf_auth_rule` VALUES (1, 1, '概况', '', 'menu.home', 1, 1, 0, 'icon-dashboard', '/home', 'home', '/dashboard/workplace/index', '', '', '', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-02-05 15:35:59', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (2, 1, '个人中心', '', '', 2, 1, 0, 'icon-user', '/usersetting', 'usersetting', 'system/usersetting/index.vue', '', '', '', 0, 0, 0, 1, 1, 0, 0, 0, 0, '2024-02-05 15:35:59', '2026-01-06 21:15:29', NULL);
INSERT INTO `gf_auth_rule` VALUES (3, 1, '系统设置', '', 'menu.system', 3, 0, 0, 'icon-settings', '/system', 'system', 'LAYOUT', '/system/account', '', '', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-02-05 15:35:59', '2025-09-27 22:06:57', NULL);
INSERT INTO `gf_auth_rule` VALUES (4, 1, '部门管理', '', 'system.dept.title', 2, 1, 3, '', 'dept', 'dept', '/system/dept/index', '', '', '', 0, 0, 1, 1, 2, 0, 0, 0, 0, '2024-02-05 15:35:59', '2025-10-09 21:34:51', NULL);
INSERT INTO `gf_auth_rule` VALUES (5, 1, '菜单管理', '', 'system.rule.title', 1, 1, 3, '', 'rule', 'rule', '/system/rule/index', '', '', '', 0, 0, 1, 1, 2, 0, 0, 0, 0, '2024-02-05 15:35:59', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (6, 1, '账户管理', '', 'system.account.title', 4, 1, 3, '', 'account', 'account', '/system/account/index', '', '', '', 0, 0, 1, 1, 0, 0, 0, 0, 0, '2024-02-05 15:35:59', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (7, 1, '角色管理', '', 'system.role.title', 3, 1, 3, '', 'role', 'role', '/system/role/index', '', '', '', 0, 0, 1, 1, 0, 0, 0, 0, 0, '2024-02-05 15:35:59', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (8, 1, '数据中心', '', 'menu.datacenter', 4, 0, 0, 'icon-storage', '/datacenter', 'datacenter', 'LAYOUT', '/datacenter/dictionary', '', '', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-02-05 15:35:59', '2025-09-27 22:06:57', NULL);
INSERT INTO `gf_auth_rule` VALUES (9, 1, '字典数据', '', 'datacenter.data.title', 9, 1, 8, '', 'data', 'data', '/datacenter/dictionary/index', '', '', '', 0, 0, 1, 1, 0, 0, 0, 0, 0, '2024-02-05 15:35:59', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (10, 1, '配置管理', '', 'datacenter.configuration.title', 12, 1, 8, '', 'configuration', 'configuration', '/datacenter/configuration/index', '', '', '', 0, 0, 1, 1, 0, 0, 0, 0, 0, '2024-02-05 15:35:59', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (11, 1, '开发者工具', '', '', 5, 0, 0, 'icon-code', '/developer', 'developer', 'LAYOUT', '/developer/codestore', '', '', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-02-05 15:35:59', '2025-09-27 22:06:57', NULL);
INSERT INTO `gf_auth_rule` VALUES (13, 1, '代码仓库', '', '', 2, 1, 11, '', 'codestore', 'codestore', '/developer/codestore/index', '', '', '', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-02-05 15:35:59', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (16, 1, '新建', '创建部门权限', '', 2, 2, 4, '', '', '', '', '', '/admin/system/dept/save', 'add', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-02-05 15:35:59', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (26, 1, '查看', '查看列表数据', '', 1, 2, 4, '', '', '', '', '', '/admin/system/dept/getList', NULL, 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-17 21:14:45', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (27, 1, '删除', '', '', 3, 2, 4, '', '', '', '', '', '/admin/system/dept/del', NULL, 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-17 22:07:22', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (28, 1, '状态', '', '', 4, 2, 4, '', '', '', '', '', '/admin/system/dept/upStatus', NULL, 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-17 22:11:12', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (32, 1, '删除', '', '', 3, 2, 7, '', '', '', '', '', '/admin/system/role/del', 'del', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-17 23:17:47', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (33, 1, '添加/编辑', '', '', 2, 2, 9, '', '', '', '', '', '/admin/datacenter/dictionary/save', 'add', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-18 14:42:07', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (34, 1, '查看', '', '', 1, 2, 9, '', '', '', '', '', '/admin/datacenter/dictionary/getList', 'view', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-18 14:45:06', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (35, 1, '状态', '', '', 4, 2, 7, '', '', '', '', '', '/admin/system/role/upStatus', 'upStatus', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-18 14:46:19', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (36, 1, '删除', '', '', 3, 2, 9, '', '', '', '', '', '/admin/datacenter/dictionary/del', 'del', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-18 15:16:15', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (37, 1, '状态', '', '', 4, 2, 9, '', '', '', '', '', '/admin/datacenter/dictionary/upStatus', 'upStatus', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-18 15:16:47', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (38, 1, '添加分组', '', '', 5, 2, 9, '', '', '', '', '', '/admin/datacenter/tabledata/save', 'addcate', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-18 15:20:32', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (39, 1, '删除分组', '', '', 6, 2, 9, '', '', '', '', '', '/admin/datacenter/tabledata/del', 'delcate', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-18 15:20:54', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (40, 1, '邮箱配置', '', '', 2, 2, 10, '', '', '', '', '', '/admin/datacenter/configuration/getEmail', NULL, 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-18 15:38:05', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (41, 1, '动态配置', '', '', 3, 2, 10, '', '', '', '', '', '/admin/datacenter/configuration/getCodestoreConfig', NULL, 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-18 15:41:07', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (42, 1, '配置状态', '', '', 4, 2, 10, '', '', '', '', '', '/admin/datacenter/configuration/upConfigStatus', NULL, 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-18 15:42:00', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (43, 1, '修改邮箱', '', '', 5, 2, 10, '', '', '', '', '', '/admin/datacenter/configuration/saveEmail', NULL, 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-18 15:43:23', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (44, 1, '修改动态配置', '', '', 6, 2, 10, '', '', '', '', '', '/admin/datacenter/configuration/saveCodeStoreConfig', NULL, 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-18 15:45:12', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (45, 1, '修改系统配置', '', '', 7, 2, 10, '', '', '', '', '', '/admin/datacenter/common_config/saveConfig', NULL, 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-18 15:57:48', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (46, 1, '系统配置', '', '', 46, 2, 10, '', '', '', '', '', '/datacenter/configuration/getCodestoreConfig', 'syscnf', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2026-03-08 14:27:56', '2026-03-08 14:27:56', NULL);
INSERT INTO `gf_auth_rule` VALUES (56, 1, '用户信息', '获取用户信息', '', 56, 2, 2, '', '', '', '', '', '/admin/user/setting/getUserinfo', NULL, 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-19 18:06:21', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (57, 1, '修改', '修改密码、手机号等用户信息', '', 57, 2, 2, '', '', '', '', '', '/admin/user/setting/saveInfo', NULL, 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-19 18:07:40', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (60, 1, '系统日志', '', '', 60, 1, 3, '', 'log', 'log', '/system/log/index', '', '', '', 0, 0, 1, 1, 0, 0, 0, 0, 0, '2024-12-19 20:53:01', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (65, 1, '登录日志', '查看登录日志', '', 65, 2, 60, '', '', '', '', '', '/admin/system/log/getLogin', 'view', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-20 19:15:12', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (66, 1, '操作日志', '', '', 66, 2, 60, '', '', '', '', '', '/admin/system/log/getOperation', NULL, 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-20 23:18:06', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (67, 1, '操作日志详情', '', '', 67, 2, 60, '', '', '', '', '', '/admin/system/log/getOperationDetail', NULL, 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-21 18:12:22', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (68, 1, '删除登录日志', '', '', 68, 2, 60, '', '', '', '', '', '/admin/system/log/delLastLogin', 'delLastLogin', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-21 20:06:27', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (69, 1, '删除操作日志', '', '', 69, 2, 60, '', '', '', '', '', '/admin/system/log/delLastOperation', 'delLastOperation', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-21 20:07:05', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (71, 1, '分类列表', '', '', 71, 2, 9, '', '', '', '', '', '/admin/datacenter/tabledata/getList', NULL, 0, 0, 0, 1, 0, 0, 0, 0, 0, '2024-12-26 22:30:16', NULL, NULL);
INSERT INTO `gf_auth_rule` VALUES (77, 1, '清空回收站', '一键清空回收站数据', '', 77, 2, 5, '', '', '', '', '', '/admin/system/rule/emptyRecyclebin', 'emptyRecyclebin', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2025-10-05 18:28:15', '2025-10-05 18:28:15', NULL);
INSERT INTO `gf_auth_rule` VALUES (78, 1, '菜单添加/编辑', '', '', 78, 2, 5, '', '', '', '', '', '/admin/system/rule/save', 'add', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2026-02-25 21:45:00', '2026-02-25 21:45:00', NULL);
INSERT INTO `gf_auth_rule` VALUES (80, 1, '上传附件', '后台系统上传附件', '', 80, 2, 6, '', '', '', '', '', '/admin/datacenter/upfile/upload', '', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2025-10-13 20:45:01', '2025-10-13 20:46:14', NULL);
INSERT INTO `gf_auth_rule` VALUES (81, 1, '查看文件上传', '', '', 81, 2, 10, '', '', '', '', '', '/admin/datacenter/uploadconfig/getConfig', '', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2025-10-14 09:42:42', '2025-10-14 09:42:42', NULL);
INSERT INTO `gf_auth_rule` VALUES (82, 1, '修改上传配置', '', '', 82, 2, 10, '', '', '', '', '', '/admin/datacenter/uploadconfig/saveConfig', '', 0, 0, 0, 1, 0, 0, 0, 0, 0, '2025-10-14 09:44:18', '2025-10-14 09:44:18', NULL);

-- ----------------------------
-- Table structure for gf_dictionary_data
-- ----------------------------
DROP TABLE IF EXISTS `gf_dictionary_data`;
CREATE TABLE `gf_dictionary_data`  (
  `id` int(10) UNSIGNED NOT NULL AUTO_INCREMENT,
  `data_from` enum('common','business') CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'common' COMMENT '数据来源:common=公共,business=商业端',
  `group_id` int(10) NOT NULL DEFAULT 0 COMMENT '数据分组id',
  `keyname` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '字典名称',
  `keyvalue` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '字典项值',
  `tagcolor` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '标签颜色',
  `des` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '字典描述',
  `status` tinyint(1) NOT NULL DEFAULT 0 COMMENT '状态',
  `weigh` int(11) NOT NULL DEFAULT 0 COMMENT '排序',
  `createtime` datetime NULL DEFAULT NULL COMMENT '创建时间',
  `updatetime` datetime NULL DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 18 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_general_ci COMMENT = '字典数据-公共表' ROW_FORMAT = DYNAMIC;

-- ----------------------------
-- Records of gf_dictionary_data
-- ----------------------------
INSERT INTO `gf_dictionary_data` VALUES (1, 'business', 2, '管理层', 'mteam', '#D91AD9', '公司领导', 0, 1, '2024-02-05 15:35:59', '2025-10-29 22:10:47');
INSERT INTO `gf_dictionary_data` VALUES (2, 'business', 2, '业务员', 'salesman', 'orange', '员工', 0, 2, '2024-02-05 15:35:59', '2025-10-29 22:10:40');
INSERT INTO `gf_dictionary_data` VALUES (5, 'business', 4, '汽车', 'car', '#00B42A', '', 0, 5, '2024-06-30 17:25:54', '2024-07-02 21:55:02');
INSERT INTO `gf_dictionary_data` VALUES (6, 'business', 4, '飞机', 'air', '#3C7EFF', '', 0, 6, '2024-06-30 22:25:44', '2024-07-02 21:54:51');

-- ----------------------------
-- Table structure for gf_dictionary_group
-- ----------------------------
DROP TABLE IF EXISTS `gf_dictionary_group`;
CREATE TABLE `gf_dictionary_group`  (
  `id` int(10) UNSIGNED NOT NULL AUTO_INCREMENT,
  `business_id` int(11) NOT NULL DEFAULT 0 COMMENT '业务主账号id',
  `title` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '字典分组名称',
  `remark` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '备注',
  `data_from` enum('common','business') CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'common' COMMENT '数据来源:common=公共,business=商业端',
  `db_way` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'sys' COMMENT '数据存储位置:sys=公共表,alone=单独建表',
  `tablename` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '数据表名称',
  `status` tinyint(1) NOT NULL DEFAULT 0 COMMENT '状态',
  `weigh` int(11) NOT NULL DEFAULT 1 COMMENT '排序',
  `createtime` datetime NULL DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 17 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_general_ci COMMENT = '字典分组' ROW_FORMAT = DYNAMIC;

-- ----------------------------
-- Records of gf_dictionary_group
-- ----------------------------
INSERT INTO `gf_dictionary_group` VALUES (2, 1, '用户分组', '用户分组', 'business', 'sys', 'dictionary_data', 0, 2, '2024-02-05 15:35:59');
INSERT INTO `gf_dictionary_group` VALUES (4, 1, '出行方式', '用来存储出行字段', 'business', 'sys', 'dictionary_data', 0, 4, '2024-06-30 16:54:36');

-- ----------------------------
-- Table structure for gf_email
-- ----------------------------
DROP TABLE IF EXISTS `gf_email`;
CREATE TABLE `gf_email`  (
  `id` int(10) UNSIGNED NOT NULL AUTO_INCREMENT,
  `data_from` enum('sys') CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'sys' COMMENT '数据来源sys=后台管理',
  `business_id` int(11) NOT NULL DEFAULT 0 COMMENT '业务主账号id',
  `sender_email` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '发送者邮箱',
  `auth_code` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '邮箱授权码',
  `mail_title` varchar(80) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '邮件标题',
  `mail_body` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '邮件内容,可以是html',
  `service_host` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '邮件服务器',
  `service_port` int(11) NOT NULL DEFAULT 0 COMMENT '邮件服务器端口',
  PRIMARY KEY (`id`) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 3 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_general_ci COMMENT = '服务邮箱' ROW_FORMAT = DYNAMIC;

-- ----------------------------
-- Records of gf_email
-- ----------------------------

-- ----------------------------
-- Table structure for gf_home_quickop
-- ----------------------------
DROP TABLE IF EXISTS `gf_home_quickop`;
CREATE TABLE `gf_home_quickop`  (
  `id` int(10) UNSIGNED NOT NULL AUTO_INCREMENT,
  `business_id` int(11) NOT NULL DEFAULT 0 COMMENT '业务主账号id',
  `uid` int(11) NOT NULL DEFAULT 0 COMMENT '添加人',
  `is_common` tinyint(1) NOT NULL DEFAULT 0 COMMENT '公共1=是',
  `type` tinyint(1) NOT NULL DEFAULT 0 COMMENT '类型1=外部',
  `name` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '快捷名称',
  `path_url` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '跳转路径',
  `icon` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '图标',
  `weigh` int(11) NOT NULL DEFAULT 0 COMMENT '权重',
  PRIMARY KEY (`id`) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 3 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_general_ci COMMENT = '首页快捷操作' ROW_FORMAT = DYNAMIC;

-- ----------------------------
-- Records of gf_home_quickop
-- ----------------------------
INSERT INTO `gf_home_quickop` VALUES (1, 1, 1, 0, 0, '系统配置', 'configuration', 'svgfont-caozuo-banli', 1);
INSERT INTO `gf_home_quickop` VALUES (2, 1, 1, 0, 0, '插件市场', 'codestore', 'icon-code-sandbox', 2);
INSERT INTO `gf_home_quickop` VALUES (3, 1, 1, 0, 0, 'AI / MCP', 'developer_ai', 'icon-code', 3);

-- ----------------------------
-- Table structure for gf_login_log
-- ----------------------------
DROP TABLE IF EXISTS `gf_login_log`;
CREATE TABLE `gf_login_log`  (
  `id` int(10) UNSIGNED NOT NULL AUTO_INCREMENT,
  `uid` int(11) NOT NULL DEFAULT 0 COMMENT '用户id',
  `username` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '登录账号',
  `ip` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '登录IP',
  `address` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '地点',
  `des` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '登录行为',
  `os` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT NULL COMMENT '操作系统',
  `browser` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT NULL COMMENT '浏览器类型',
  `error_msg` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '登录失败原因',
  `status` tinyint(1) NOT NULL DEFAULT 0 COMMENT '状态:0=成功,1=失败',
  `createtime` datetime NULL DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 1 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_general_ci COMMENT = '系统登录日志' ROW_FORMAT = DYNAMIC;

-- ----------------------------
-- Records of gf_login_log
-- ----------------------------

-- ----------------------------
-- Table structure for gf_member
-- ----------------------------
DROP TABLE IF EXISTS `gf_member`;
CREATE TABLE `gf_member`  (
  `id` int(10) UNSIGNED NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `business_id` int(11) NOT NULL DEFAULT 1 COMMENT '业务主账号id',
  `username` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT '' COMMENT '用户名',
  `name` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '姓名',
  `nickname` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT '' COMMENT '昵称',
  `remark` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '备注',
  `password` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT '' COMMENT '密码',
  `salt` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT '' COMMENT '密码盐',
  `email` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT '' COMMENT '电子邮箱',
  `mobile` varchar(11) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT '' COMMENT '手机号',
  `avatar` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT '' COMMENT '头像',
  `level` tinyint(3) UNSIGNED NOT NULL DEFAULT 0 COMMENT '等级',
  `sex` tinyint(3) UNSIGNED NOT NULL DEFAULT 0 COMMENT '性别:1=男性,2=女性,0=未知',
  `birthday` date NULL DEFAULT NULL COMMENT '出生日期',
  `money` decimal(10, 2) NOT NULL COMMENT '余额',
  `score` int(11) NOT NULL DEFAULT 0 COMMENT '积分',
  `successions` int(10) UNSIGNED NOT NULL DEFAULT 1 COMMENT '连续登录天数',
  `maxsuccessions` int(10) UNSIGNED NOT NULL DEFAULT 1 COMMENT '最大连续登录天数',
  `prevtime` bigint(20) NULL DEFAULT NULL COMMENT '上次登录时间',
  `logintime` bigint(20) NULL DEFAULT NULL COMMENT '登录时间',
  `loginip` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT NULL COMMENT '登录IP',
  `loginfailure` tinyint(3) UNSIGNED NOT NULL DEFAULT 0 COMMENT '失败次数',
  `status` tinyint(1) NOT NULL DEFAULT 0 COMMENT '状态',
  `createtime` datetime NULL DEFAULT NULL COMMENT '创建时间',
  `updatetime` datetime NULL DEFAULT NULL COMMENT '更新时间',
  `deletetime` datetime NULL DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `username`(`username` ASC) USING BTREE,
  INDEX `email`(`email` ASC) USING BTREE,
  INDEX `mobile`(`mobile` ASC) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 5 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_general_ci COMMENT = '会员表(用户主表)' ROW_FORMAT = DYNAMIC;

-- ----------------------------
-- Records of gf_member
-- ----------------------------

-- ----------------------------
-- Table structure for gf_operation_log
-- ----------------------------
DROP TABLE IF EXISTS `gf_operation_log`;
CREATE TABLE `gf_operation_log`  (
  `id` int(10) UNSIGNED NOT NULL AUTO_INCREMENT,
  `business_id` int(11) NOT NULL DEFAULT 0 COMMENT '业务主账号id',
  `uid` int(11) NOT NULL DEFAULT 0 COMMENT '用户id',
  `method` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '请求方法',
  `url` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '请求地址',
  `ip` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '登录IP',
  `address` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '地点',
  `des` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '登录行为',
  `req_headers` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '请求头',
  `req_body` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '请求体',
  `resp_headers` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '响应头',
  `resp_body` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '响应体',
  `latency` decimal(10, 4) NOT NULL DEFAULT 0.0000 COMMENT '耗时',
  `status` int(2) NOT NULL DEFAULT 0 COMMENT '状态:0=成功,1=失败',
  `createtime` datetime NULL DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 1 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_general_ci COMMENT = '系统操作日志' ROW_FORMAT = DYNAMIC;

-- ----------------------------
-- Records of gf_operation_log
-- ----------------------------

SET FOREIGN_KEY_CHECKS = 1;

-- Add the connection-test permission alongside the existing upload settings.
-- Existing installations can run this file once or repeatedly.
INSERT INTO gf_auth_rule
 (uid,title,des,locale,weigh,type,pid,icon,routepath,routename,component,redirect,path,permission,status,isext,keepalive,requiresauth,hideinmenu,hidechildreninmenu,activemenu,noaffix,onlypage,createtime)
SELECT uid,'测试123云盘连接','测试上传与直链访问','',weigh+1,2,pid,'','','','','',
 '/admin/datacenter/uploadconfig/testConnection','pan123Test',0,0,0,1,0,0,0,0,0,NOW()
FROM gf_auth_rule
WHERE path='/admin/datacenter/uploadconfig/saveConfig'
AND NOT EXISTS (SELECT 1 FROM (SELECT path FROM gf_auth_rule) existing WHERE existing.path='/admin/datacenter/uploadconfig/testConnection')
LIMIT 1;

-- GoSuxin V1.0.0: additive and idempotent application metadata migration.
CREATE TABLE IF NOT EXISTS `gf_app_meta` (
  `meta_key` varchar(64) NOT NULL,
  `meta_value` varchar(255) NOT NULL,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`meta_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
INSERT INTO `gf_app_meta` (`meta_key`, `meta_value`) VALUES ('app_version', '1.0.0')
ON DUPLICATE KEY UPDATE `meta_value` = VALUES(`meta_value`);

-- Administrator cannot authenticate until the installer writes its chosen password.
INSERT INTO gf_admin (id,account_id,dept_id,username,password,salt,name,nickname,avatar,email,mobile,tel,status,remark,loginip,login_attempts)
VALUES (1,0,1,'admin','','','Suxin管理员','Suxin管理员','/resource/static/brand/logo.png?v=suxin1','','','',0,'','',0);
INSERT INTO gf_auth_dept (id,account_id,pid,name,weigh,status,remark)
VALUES (1,1,0,'Suxin技术团队',1,0,'');
