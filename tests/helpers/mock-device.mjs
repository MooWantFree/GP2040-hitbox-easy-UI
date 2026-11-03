import { createServer } from 'node:http';
import { mkdir, writeFile } from 'node:fs/promises';
const resultsDirectory = new URL('../../.qa/', import.meta.url);
await mkdir(resultsDirectory, { recursive: true });
const profile = (name) => ({ ...Object.fromEntries(Array.from({ length: 30 }, (_, i) => [`pin${String(i).padStart(2, '0')}`, { action: i < 2 ? -5 : Math.min(i + 1,18), customButtonMask: 0, customDpadMask: 0 }])), profileLabel: name, enabled: true, pinInversionEnabled: false, invertedPins: [] });
let base = profile('默认配置'), alternatives = [profile('训练配置')], writes = 0, captures = [[4],[5],[6,7],[5],[6],[]];
const server=createServer(async(req,res)=>{
 try {
  const chunks=[];for await (const c of req) chunks.push(c);const data=chunks.length?JSON.parse(Buffer.concat(chunks)):null;
  let out;switch(req.url){
   case '/api/getFirmwareVersion':out={version:'模拟设备 · 不连接硬件',boardBuild:'浏览器测试'};break;
   case '/api/getPinMappings':out=base;break;
   case '/api/getProfileOptions':out={alternativePinMappings:alternatives};break;
   case '/api/getGamepadOptions':out={profileNumber:1};break;
   case '/api/getConfig':out={gpioMappings:base,profileOptions:alternatives,testOnly:true};break;
   case '/api/getHeldPins':await new Promise(r=>setTimeout(r,350));out={heldPins:captures.shift()||[]};break;
   case '/api/setPinMappings':base=data;writes++;out=data;break;
   case '/api/setProfileOptions':alternatives=data.alternativePinMappings;writes++;out=data;break;
   case '/test/state':out={base,alternatives,writes};break;
   case '/test/captures':captures=data;out={ok:true};break;
   default:res.writeHead(404);res.end('{}');return;
  }
  await writeFile(new URL('go-ui-results.json',resultsDirectory),JSON.stringify({base,alternatives,writes},null,2));
  res.setHeader('Content-Type','application/json');res.end(JSON.stringify(out));
 }catch(e){res.writeHead(500);res.end(JSON.stringify({error:e.message}));}
});
server.listen(8099,'127.0.0.1',()=>console.log('Mock hardware: 127.0.0.1:8099'));
