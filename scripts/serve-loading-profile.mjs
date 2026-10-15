// Isolated localhost development server. Counters run only on this server;
// regular development/builds continue using the uninstrumented worker.
import {readFile,writeFile,rename} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
const root=fileURLToPath(new URL('../',import.meta.url));
const {createServer}=await import(new URL('../web/node_modules/vite/dist/node/index.js',import.meta.url));
const hook=await readFile(new URL('./loading-profile-worker-hook.js',import.meta.url),'utf8');
const worker=await readFile(new URL('../web/public/practice/worker.js',import.meta.url),'utf8');
const plain=process.env.RIFFINE_LOADING_PROFILE_PLAIN==='1';
const server=await createServer({
 configFile:false,root:`${root}web`,
 server:{host:'127.0.0.1',port:5175,strictPort:true,fs:{allow:[root]},headers:{'Cross-Origin-Opener-Policy':'same-origin','Cross-Origin-Embedder-Policy':'require-corp'}},
 plugins:[{name:'loading-profile-worker',configureServer(server){server.middlewares.use((req,res,next)=>{
  const fixturePrefix='/__loading-profile/fixtures/';
  if(req.url?.startsWith(fixturePrefix)){
   const name=req.url.slice(fixturePrefix.length);
   if(!/^(manifest\.json|tone-\d+s\.(wav|mp3|flac|m4a|opus))$/.test(name)){res.statusCode=400;res.end('Invalid fixture name.');return;}
   void readFile(`${root}build/import-profile/${name}`).then(bytes=>{
    res.setHeader('Content-Type',name.endsWith('.json')?'application/json':'application/octet-stream');res.end(bytes);
   },()=>{res.statusCode=404;res.end('Generate loading fixtures first.');});return;
  }
  if(req.url==='/__loading-profile/result'&&req.method==='POST'){
   void(async()=>{
    try{
     let body='';for await(const chunk of req){body+=chunk;if(body.length>1_000_000)throw new Error('Profile payload exceeds bound.');}
     const profile=JSON.parse(body),path=`${root}build/import-profile/${plain?'browser-plain-results':'browser-results'}.json`;
     await writeFile(`${path}.tmp`,JSON.stringify(profile,null,2)+'\n');await rename(`${path}.tmp`,path);
     res.setHeader('Content-Type','application/json');res.end('{"saved":true}');
    }catch(error){res.statusCode=500;res.end(String(error));}
   })();return;
  }
  if(req.url?.split('?')[0]!=='/practice/worker.js')return next();
  res.setHeader('Content-Type','text/javascript');
  res.setHeader('Cross-Origin-Opener-Policy','same-origin');res.setHeader('Cross-Origin-Embedder-Policy','require-corp');
  res.end(plain?worker:`${hook}\n${worker}`);
 });}}],
});
await server.listen();server.printUrls();
