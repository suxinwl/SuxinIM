  import { Message,} from '@arco-design/web-vue';
export const toExcel = (response: any,id:string) => {
  try {
    if (response.headers['content-type'].includes('json')) {
      var reader = new FileReader();
      reader.readAsText(response.data, 'utf-8');
      reader.onload = function() {
         var result = JSON.parse(reader.result as any);
         Message.error({content:result.message,id:id,duration:2000})
      };
      return false
    }else{
      const contentDisposition = response.headers['content-disposition'];
      let filename = 'export.xlsx';
      if (contentDisposition) {
        const filenameMatch = contentDisposition.match(/filename=(.+)/);
        if (filenameMatch) filename = decodeURIComponent(filenameMatch[1]);
      }
      const blob = new Blob([response.data], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' });
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', filename);
      document.body.appendChild(link);
      link.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(link);
      return true
    }
  } catch (error) {
    throw error;
  }
};

//column数据过滤-作为导出数据字段
export const filterColumns = (data: any) => {
  let list:any=[];
   data.forEach((item:any) => {
    if(item.dataIndex!=="operations")
     list.push({title:item.title,field:item.dataIndex})
   });
   return list;
};
