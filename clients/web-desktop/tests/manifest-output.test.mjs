import test from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import {clientManifestOutput} from '../scripts/client-manifest-output.mjs';
const suite=path.resolve('../..');
test('existing public and candidate output defaults remain unchanged',()=>{
 assert.equal(clientManifestOutput(suite,[]).output,path.join(suite,'clients/artifacts-manifest.json'));
 assert.equal(clientManifestOutput(suite,['--candidate']).output,path.join(suite,'.private/production/acceptance/client-organization-candidate-artifacts.json'));
});
test('new release candidate can use a separate private name without overwriting previous metadata',()=>{
 const result=clientManifestOutput(suite,['--candidate','--candidate-output','.private/production/acceptance/attachment-20261103/client-3.8.10-candidate.json']);
 assert.equal(result.candidate,true);assert.match(result.output,/client-3\.8\.10-candidate\.json$/);
});
test('explicit candidate paths cannot write public metadata or escape acceptance',()=>{
 for(const args of [ ['--candidate-output','.private/production/acceptance/anything.json'],['--candidate','--candidate-output','clients/artifacts-manifest.json'],['--candidate','--candidate-output','.private/production/acceptance/../../escape.json'],['--candidate','--candidate-output','.private/production/acceptance/wrong.txt'],['--unknown'] ]) assert.throws(()=>clientManifestOutput(suite,args));
});
