#ifndef RIFFINE_WINDOWS_M4A_H
#define RIFFINE_WINDOWS_M4A_H
#ifdef _WIN32
#include "encoded_file.h"
#include <mfapi.h>
#include <mfidl.h>
#include <mfreadwrite.h>
#include <wrl/client.h>
#include <atomic>
#include <algorithm>
#include <cstdio>
#include <cstring>
#include <memory>
#include <mutex>
#include <stdexcept>
#include <vector>

namespace riffine {
using Microsoft::WRL::ComPtr;

inline void mediaCheck(HRESULT result, const char *operation) {
 if(FAILED(result)) {
  char text[384];
  std::snprintf(text,sizeof(text),"%s (Windows media error 0x%08lx). M4A requires unprotected AAC or Apple Lossless audio and Windows media components.",operation,static_cast<unsigned long>(result));
  throw std::runtime_error(text);
 }
}
inline void mediaRuntime() {
 // Source readers can be opened and consumed on different worker threads.
 struct Apartment {
  HRESULT result=CoInitializeEx(nullptr,COINIT_MULTITHREADED);
  Apartment(){if(result!=RPC_E_CHANGED_MODE)mediaCheck(result,"Cannot initialize Windows audio decoder");}
  ~Apartment(){if(SUCCEEDED(result))CoUninitialize();}
 };
 thread_local Apartment apartment;
 struct Runtime {
  Runtime(){mediaCheck(MFStartup(MF_VERSION),"Cannot start Windows media components");}
  ~Runtime(){MFShutdown();}
 };
 static Runtime runtime;
}

// Read-only COM stream over the SAME retained resource used for fingerprinting.
// No pathname lookup, temporary conversion, or whole-song encoded/PCM buffer.
// Clones have independent cursors; the overlapped read event is serialized.
class EncodedStream final : public IStream {
 struct Shared {
  std::shared_ptr<EncodedFile> file;
  std::mutex mutex;
  explicit Shared(std::shared_ptr<EncodedFile> file):file(std::move(file)){}
 };
 std::shared_ptr<Shared> shared;
 std::atomic<ULONG> references{1};
 uint64_t position=0;
 std::mutex cursor;
 explicit EncodedStream(std::shared_ptr<Shared> shared,uint64_t position):shared(std::move(shared)),position(position){}
public:
 explicit EncodedStream(std::shared_ptr<EncodedFile> file):shared(std::make_shared<Shared>(std::move(file))){}
 HRESULT STDMETHODCALLTYPE QueryInterface(REFIID id,void **out) override {
  if(!out)return E_POINTER;
  *out=nullptr;
  if(id!=IID_IUnknown&&id!=IID_ISequentialStream&&id!=IID_IStream)return E_NOINTERFACE;
  *out=static_cast<IStream *>(this);AddRef();return S_OK;
 }
 ULONG STDMETHODCALLTYPE AddRef() override{return ++references;}
 ULONG STDMETHODCALLTYPE Release() override{auto n=--references;if(!n)delete this;return n;}
 HRESULT STDMETHODCALLTYPE Read(void *out,ULONG bytes,ULONG *read) override {
  if(read)*read=0;
  if(!out&&bytes)return STG_E_INVALIDPOINTER;
  std::scoped_lock lock(cursor,shared->mutex);
  ULONG done=0;
  try {
   while(done<bytes){auto n=shared->file->readAt(static_cast<unsigned char *>(out)+done,bytes-done,position);if(!n)break;position+=n;done+=ULONG(n);}
  }catch(...){if(read)*read=done;return STG_E_READFAULT;}
  if(read)*read=done;
  return done==bytes?S_OK:S_FALSE;
 }
 HRESULT STDMETHODCALLTYPE Seek(LARGE_INTEGER offset,DWORD origin,ULARGE_INTEGER *result) override {
  std::lock_guard<std::mutex> lock(cursor);
  if(origin>STREAM_SEEK_END)return STG_E_INVALIDFUNCTION;
  uint64_t base=origin==STREAM_SEEK_SET?0:origin==STREAM_SEEK_CUR?position:shared->file->size(),next;
  if(offset.QuadPart<0){auto n=uint64_t(-(offset.QuadPart+1))+1;if(n>base)return STG_E_INVALIDFUNCTION;next=base-n;}
  else{if(uint64_t(offset.QuadPart)>UINT64_MAX-base)return STG_E_INVALIDFUNCTION;next=base+uint64_t(offset.QuadPart);}
  position=next;if(result)result->QuadPart=position;return S_OK;
 }
 HRESULT STDMETHODCALLTYPE Stat(STATSTG *stat,DWORD) override {
  if(!stat)return E_POINTER;
  *stat={};stat->type=STGTY_STREAM;stat->cbSize.QuadPart=shared->file->size();stat->grfMode=STGM_READ;return S_OK;
 }
 HRESULT STDMETHODCALLTYPE Clone(IStream **out) override {
  if(!out)return E_POINTER;
  *out=nullptr;
  std::lock_guard<std::mutex> lock(cursor);
  try{*out=new EncodedStream(shared,position);return S_OK;}catch(...){return E_OUTOFMEMORY;}
 }
 HRESULT STDMETHODCALLTYPE Write(const void *,ULONG,ULONG *) override{return STG_E_ACCESSDENIED;}
 HRESULT STDMETHODCALLTYPE SetSize(ULARGE_INTEGER) override{return STG_E_ACCESSDENIED;}
 HRESULT STDMETHODCALLTYPE CopyTo(IStream *,ULARGE_INTEGER,ULARGE_INTEGER *,ULARGE_INTEGER *) override{return E_NOTIMPL;}
 HRESULT STDMETHODCALLTYPE Commit(DWORD) override{return S_OK;}
 HRESULT STDMETHODCALLTYPE Revert() override{return STG_E_INVALIDFUNCTION;}
 HRESULT STDMETHODCALLTYPE LockRegion(ULARGE_INTEGER,ULARGE_INTEGER,DWORD) override{return STG_E_INVALIDFUNCTION;}
 HRESULT STDMETHODCALLTYPE UnlockRegion(ULARGE_INTEGER,ULARGE_INTEGER,DWORD) override{return STG_E_INVALIDFUNCTION;}
};

class WindowsM4A {
 std::shared_ptr<EncodedFile> resource;
 ComPtr<IMFSourceReader> reader;
 std::vector<float> pending;
 size_t offset=0;
 bool ended=false;
 void open() {
  mediaRuntime();reader.Reset();pending.clear();offset=0;ended=false;
  ComPtr<IStream> stream;stream.Attach(new EncodedStream(resource));
  ComPtr<IMFByteStream> bytes;
  mediaCheck(MFCreateMFByteStreamOnStream(stream.Get(),&bytes),"Cannot read M4A resource");
  mediaCheck(MFCreateSourceReaderFromByteStream(bytes.Get(),nullptr,&reader),"Cannot open M4A");
  mediaCheck(reader->SetStreamSelection(MF_SOURCE_READER_ALL_STREAMS,FALSE),"Cannot select M4A audio");
  mediaCheck(reader->SetStreamSelection(MF_SOURCE_READER_FIRST_AUDIO_STREAM,TRUE),"M4A has no audio stream");
  ComPtr<IMFMediaType> native;
  mediaCheck(reader->GetNativeMediaType(MF_SOURCE_READER_FIRST_AUDIO_STREAM,0,&native),"Cannot inspect M4A audio");
  GUID subtype{};mediaCheck(native->GetGUID(MF_MT_SUBTYPE,&subtype),"Missing M4A codec");
  uint32_t detected=subtype==MFAudioFormat_AAC?4:subtype==MFAudioFormat_ALAC?5:0;
  if(!detected)throw std::runtime_error("Unsupported M4A audio codec. Choose unprotected AAC or Apple Lossless audio.");
  if(encoding&&detected!=encoding)throw std::runtime_error("M4A source codec changed");
  encoding=detected;
  ComPtr<IMFMediaType> output;
  mediaCheck(MFCreateMediaType(&output),"Cannot configure M4A audio");
  mediaCheck(output->SetGUID(MF_MT_MAJOR_TYPE,MFMediaType_Audio),"Cannot configure M4A audio");
  mediaCheck(output->SetGUID(MF_MT_SUBTYPE,MFAudioFormat_Float),"Cannot configure M4A PCM");
  mediaCheck(reader->SetCurrentMediaType(MF_SOURCE_READER_FIRST_AUDIO_STREAM,nullptr,output.Get()),"Cannot decode M4A audio");
  mediaCheck(reader->GetCurrentMediaType(MF_SOURCE_READER_FIRST_AUDIO_STREAM,&output),"Cannot inspect decoded M4A");
  UINT32 rate=0,count=0,bits=0;
  mediaCheck(output->GetUINT32(MF_MT_AUDIO_SAMPLES_PER_SECOND,&rate),"Missing M4A sample rate");
  mediaCheck(output->GetUINT32(MF_MT_AUDIO_NUM_CHANNELS,&count),"Missing M4A channels");
  mediaCheck(output->GetUINT32(MF_MT_AUDIO_BITS_PER_SAMPLE,&bits),"Missing M4A PCM precision");
  if(count<1||count>2||rate<8000||rate>192000||bits!=32)throw std::runtime_error("M4A requires mono/stereo audio at 8-192 kHz");
  if(channels&&(channels!=count||sampleRate!=rate))throw std::runtime_error("M4A audio format changed");
  channels=count;sampleRate=rate;
 }
 bool refill() {
  pending.clear();offset=0;
  // Bound empty/tick samples so a malformed source cannot loop indefinitely.
  for(unsigned attempt=0;attempt<4096&&!ended;++attempt) {
   DWORD flags=0;ComPtr<IMFSample> sample;
   mediaCheck(reader->ReadSample(MF_SOURCE_READER_FIRST_AUDIO_STREAM,0,nullptr,&flags,nullptr,&sample),"Cannot decode M4A sample");
   if(flags&(MF_SOURCE_READERF_ERROR|MF_SOURCE_READERF_CURRENTMEDIATYPECHANGED))throw std::runtime_error("M4A decoder failed or changed audio format");
   ended=(flags&MF_SOURCE_READERF_ENDOFSTREAM)!=0;
   if(!sample)continue;
   DWORD total=0;mediaCheck(sample->GetTotalLength(&total),"Cannot inspect M4A sample");
   if(total>1024*1024||total%(channels*sizeof(float)))throw std::runtime_error("Invalid or oversized M4A decoded sample");
   if(!total)continue;
   ComPtr<IMFMediaBuffer> buffer;mediaCheck(sample->ConvertToContiguousBuffer(&buffer),"Cannot read M4A PCM");
   pending.resize(total/sizeof(float));
   BYTE *data=nullptr;DWORD length=0;
   mediaCheck(buffer->Lock(&data,nullptr,&length),"Cannot access M4A PCM");
   if(length!=total){buffer->Unlock();throw std::runtime_error("Invalid M4A PCM buffer length");}
   std::memcpy(pending.data(),data,length);buffer->Unlock();return true;
  }
  if(!ended)throw std::runtime_error("M4A decoder made no audio progress");
  return false;
 }
public:
 uint32_t channels=0,sampleRate=0,encoding=0;
 uint64_t frames=0;
 explicit WindowsM4A(std::shared_ptr<EncodedFile> resource):resource(std::move(resource)) {
  open();
  // Container duration is not necessarily the decoded length (AAC padding and
  // MP4 edit lists). Count bounded blocks once; never retain whole-song PCM.
  while(refill()) {
   frames+=pending.size()/channels;
   if(frames>(uint64_t(1)<<48))throw std::runtime_error("M4A source is too long");
  }
  if(!frames)throw std::runtime_error("M4A contains no decoded audio");
  open();
 }
 uint32_t read(float *output,uint32_t capacity) {
  mediaRuntime();uint32_t done=0;
  while(done<capacity) {
   if(offset==pending.size()&&!refill())break;
   auto n=std::min<size_t>(capacity-done,(pending.size()-offset)/channels);
   std::copy_n(pending.data()+offset,n*channels,output+size_t(done)*channels);
   offset+=n*channels;done+=uint32_t(n);
  }
  return done;
 }
 void rewind(){open();}
};
}
#endif
#endif
