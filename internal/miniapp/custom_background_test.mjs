import test from "node:test";
import assert from "node:assert/strict";
import { backgroundGeometry, draggedPosition, disposeGIFFrame, gifDelay, syncCustomBackground } from "./static/custom-background.mjs";

test("portrait and desktop framing preserve aspect ratio and drag direction", () => {
  const settings = {fit:"cover",scale:100,positionX:50,positionY:50};
  assert.deepEqual(backgroundGeometry(390,780,900,1200,settings), {width:585,height:780,left:-97.5,top:0});
  const contained = backgroundGeometry(390,780,900,1200,{...settings,fit:"contain"});
  assert.equal(contained.width,390);assert.equal(contained.height,520);assert.equal(contained.top,130);
  assert.equal(draggedPosition(50,20,390,585),50-20/195*100);
  assert.equal(draggedPosition(50,26,780,520),60);
  assert.equal(draggedPosition(50,26,780,780),50);
  assert.equal(draggedPosition(95,200,780,520),100);
  const wide = backgroundGeometry(1280,720,900,1200,{...settings,scale:150,positionY:0});
  assert.equal(wide.width,1920);assert.equal(wide.height,2560);assert.equal(Math.abs(wide.top),0);
});

test("GIF disposal clears only its previous rectangle or restores the saved frame", () => {
  const calls=[];
  const ctx={clearRect:(...args)=>calls.push(["clear",...args]),putImageData:(...args)=>calls.push(["restore",...args])};
  disposeGIFFrame(ctx,{disposalType:2,dims:{left:3,top:4,width:5,height:6}},null);
  const saved={pixels:"previous"};disposeGIFFrame(ctx,{disposalType:3},saved);
  disposeGIFFrame(ctx,{disposalType:1},saved);
  assert.deepEqual(calls,[["clear",3,4,5,6],["restore",saved,0,0]]);
  assert.equal(gifDelay({delay:1}),20);assert.equal(gifDelay({delay:0}),100);
});

test("real GIF frames use the speed slider, pause without catching up and release resources", async () => {
  const original={};
  for(const key of ["document","ResizeObserver","ImageData","requestAnimationFrame","cancelAnimationFrame","fetch"])original[key]=globalThis[key];
  let scheduled, ticket=0, aborts=0;
  const ctx={fillRect(){},clearRect(){},putImageData(){},drawImage(){},getImageData(){return {};}};
  const makeMedia=tag=>({tagName:tag.toUpperCase(),style:{},getContext:()=>ctx,remove(){},width:0,height:0});
  const host={isConnected:true,clientWidth:390,clientHeight:780,dataset:{},style:{setProperty(){}},prepend(){}};
  globalThis.document={hidden:false,createElement:makeMedia};
  globalThis.ResizeObserver=class {observe(){}disconnect(){aborts++;}};
  globalThis.ImageData=class {constructor(patch,w,h){this.patch=patch;this.width=w;this.height=h;}};
  globalThis.requestAnimationFrame=callback=>{scheduled=callback;return ++ticket;};
  globalThis.cancelAnimationFrame=()=>{scheduled=null;};
  const bytes=Buffer.from("R0lGODlhBAAEAIEAAAAAAAAAAAAAAAAAACH/C05FVFNDQVBFMi4wAwEAAAAh+QQICgAAACwAAAAABAAEAAAICQABCBxIsCCAgAAh+QQICgAAACwAAAAABAAEAIH///8AAAAAAAAAAAAICQABCBxIsCCAgAA7","base64");
  globalThis.fetch=async()=>({ok:true,arrayBuffer:async()=>bytes.buffer.slice(bytes.byteOffset,bytes.byteOffset+bytes.byteLength)});
  const settings={url:"/gif-test",type:"gif",scale:100,speed:200};
  try {
    const controller=syncCustomBackground(host,settings);
    for(let attempts=0;!controller.gif&&attempts<100;attempts++)await new Promise(resolve=>setTimeout(resolve,2));
    assert.equal(controller.gif.frames.length,2);
    scheduled(1000);scheduled(1050);assert.equal(controller.frame,1,"2x advances a 100 ms frame in 50 ms");
    settings.speed=50;syncCustomBackground(host,settings);scheduled(1150);
    assert.equal(controller.frame,1,"0.5x does not advance after 100 ms");scheduled(1250);assert.equal(controller.frame,0);
    syncCustomBackground(host,settings,{paused:true});assert.equal(scheduled,null);
    syncCustomBackground(host,settings);scheduled(9000);assert.equal(controller.frame,0,"resume starts with a fresh clock");
    syncCustomBackground(host,null);assert.equal(scheduled,null);assert.equal(controller.media,null);assert.equal(aborts,1);
  } finally {
    syncCustomBackground(host,null);
    for(const [key,value] of Object.entries(original)){if(value===undefined)delete globalThis[key];else globalThis[key]=value;}
  }
});
