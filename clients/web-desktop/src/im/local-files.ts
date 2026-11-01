export const toLocalFileURL=(path:string|undefined)=>path?`openim38://app/__local?path=${encodeURIComponent(path)}`:"";
export const isLocalFileURL=(url:string|undefined):boolean=>!!url && (url.startsWith("file://") || url.startsWith("openim38://app/__local?"));
export const localFilePath=(url:string)=>url.startsWith("openim38://app/__local?")?(new URL(url).searchParams.get("path") || ""):url.replace(/^file:\/\//, "");
