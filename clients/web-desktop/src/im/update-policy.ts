export type Release = {version:string;build:number;url:string;sha256:string;size:number;force:boolean;text?:string;platform?:string;applicationID?:string};
export function compareVersions(a:string,b:string):number {
 const parse=(v:string)=>{const m=/^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$/.exec(v);if(!m)throw new Error("Invalid release version");return {numbers:m.slice(1,4).map(Number),pre:m[4]?.split(".")};};
 const x=parse(a),y=parse(b);for(let i=0;i<3;i++)if(x.numbers[i]!==y.numbers[i])return x.numbers[i]>y.numbers[i]?1:-1;
 if(!x.pre && !y.pre)return 0;if(!x.pre)return 1;if(!y.pre)return -1;
 for(let i=0;i<Math.max(x.pre.length,y.pre.length);i++){const l=x.pre[i],r=y.pre[i];if(l===undefined)return -1;if(r===undefined)return 1;if(l===r)continue;const ln=/^\d+$/.test(l),rn=/^\d+$/.test(r);if(ln && rn)return Number(l)>Number(r)?1:-1;if(ln!==rn)return ln?-1:1;return l>r?1:-1;}return 0;
}
export function shouldUpgrade(release:Release,current:string,build:number):boolean {
 if(!Number.isSafeInteger(release.build) || release.build<=build)return false;
 return compareVersions(release.version,current)>=0;
}
export function validateRelease(release:Release):void {
 if(!/^https:\/\//.test(release.url) || !/^[a-f0-9]{64}$/i.test(release.sha256) || !Number.isSafeInteger(release.size) || release.size<1024)throw new Error("Invalid signed release metadata");
 if(release.platform && release.platform!=="windows")throw new Error("Unexpected package platform");
}
