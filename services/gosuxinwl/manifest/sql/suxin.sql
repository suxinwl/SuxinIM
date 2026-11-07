-- Add GoSuxin defaults without changing existing profile or contact information.
UPDATE gf_admin SET avatar='/resource/static/brand/logo.png?v=suxin1' WHERE avatar IS NULL OR avatar='';
INSERT INTO gf_home_quickop (business_id,uid,is_common,type,name,path_url,icon,weigh)
SELECT 0,0,0,0,'AI / MCP','developer_ai','icon-code',3
WHERE NOT EXISTS (SELECT 1 FROM gf_home_quickop WHERE path_url='developer_ai');
