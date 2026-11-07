-- Add the connection-test permission alongside the existing upload settings.
-- Existing installations can run this file once or repeatedly.
ALTER TABLE gf_attachment MODIFY COLUMN filesize BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '文件大小（字节）';
INSERT INTO gf_auth_rule
 (uid,title,des,locale,weigh,type,pid,icon,routepath,routename,component,redirect,path,permission,status,isext,keepalive,requiresauth,hideinmenu,hidechildreninmenu,activemenu,noaffix,onlypage,createtime)
SELECT uid,'测试123云盘连接','测试上传与直链访问','',weigh+1,2,pid,'','','','','',
 '/admin/datacenter/uploadconfig/testConnection','pan123Test',0,0,0,1,0,0,0,0,0,NOW()
FROM gf_auth_rule
WHERE path='/admin/datacenter/uploadconfig/saveConfig'
AND NOT EXISTS (SELECT 1 FROM (SELECT path FROM gf_auth_rule) existing WHERE existing.path='/admin/datacenter/uploadconfig/testConnection')
LIMIT 1;
