// @ts-nocheck
import { getCosAuthorization } from '@/services/admin/upload';
import COS from 'cos-js-sdk-v5';
import type { UploadRequestOption } from 'rc-upload/lib/interface';

export const setCosAuthorization = async () => {
  const { data, errCode } = await getCosAuthorization();
  if (errCode === 0) localStorage.setItem(`cosprofile`, JSON.stringify(data));
};

export const cos = new COS({
  getAuthorization: function (options, callback) {
    let cosprofile = localStorage.getItem('cosprofile');
    if (!cosprofile) return;
    cosprofile = JSON.parse(cosprofile);
    const result = cosprofile.CredentialResult;

    callback({
      TmpSecretId: result.Credentials?.TmpSecretID,
      TmpSecretKey: result.Credentials?.TmpSecretKey,
      SecurityToken: result.Credentials?.SessionToken,
      StartTime: result.StartTime,
      ExpiredTime: result.ExpiredTime,
    });
  },
});

export const cosUpload = (
  data: UploadRequestOption,
  pcb?: (progress: number) => void,
): Promise<{ data: COS.PutObjectResult & { URL: string } }> => {
  return new Promise((resolve, reject) => {
    let cosprofile = localStorage.getItem('cosprofile');
    if (!cosprofile) reject('no cosprofile');
    cosprofile = JSON.parse(cosprofile);

    cos.putObject(
      {
        Bucket: cosprofile.Bucket /* 必须 */,
        Region: cosprofile.Region /* 存储桶所在地域，必须字段 */,
        //@ts-ignore
        Key: data.file.uid + data.file.name /* 必须 */,
        // StorageClass: 'STANDARD',
        Body: data.file, // 上传文件对象
        onProgress: (params) => {
          if (pcb) {
            pcb((params.percent * 100).toFixed());
          }
        },
      },
      function (cerr, cdata) {
        if (cerr) {
          reject(cerr);
        } else {
          cdata.URL = 'https://' + cdata.Location;
          const tdata = {
            data: cdata,
          };
          resolve(tdata);
        }
      },
    );
  });
};
