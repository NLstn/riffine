#ifndef RIFFINE_ENCODED_FILE_H
#define RIFFINE_ENCODED_FILE_H
#include <algorithm>
#include <cstdint>
#include <limits>
#include <stdexcept>
#ifdef _WIN32
#ifndef NOMINMAX
#define NOMINMAX
#endif
#include <windows.h>
#else
#include <cerrno>
#include <fcntl.h>
#include <sys/stat.h>
#include <unistd.h>
#endif
namespace riffine {
/* Worker-only owned duplicate/reopen of a borrowed read-only disk resource.
 * Offset reads never change the caller's file position. No pathname lookup,
 * decoder state, Go callbacks or realtime use. Caller retains its own handle. */
class EncodedFile {
#ifdef _WIN32
 HANDLE handle_=INVALID_HANDLE_VALUE,event_=nullptr;
#else
 int handle_=-1;
#endif
 uint64_t size_=0,position_=0;
 static void require(bool ok,const char *text){if(!ok)throw std::runtime_error(text);}
 void close() noexcept {
#ifdef _WIN32
  if(event_)CloseHandle(event_);event_=nullptr;
  if(handle_!=INVALID_HANDLE_VALUE)CloseHandle(handle_);handle_=INVALID_HANDLE_VALUE;
#else
  if(handle_>=0)::close(handle_);handle_=-1;
#endif
 }
public:
 explicit EncodedFile(uint64_t borrowed){
  try{
#ifdef _WIN32
   require(borrowed<=uint64_t(UINTPTR_MAX),"invalid borrowed file handle");
   auto original=reinterpret_cast<HANDLE>(uintptr_t(borrowed));
   BY_HANDLE_FILE_INFORMATION info{};
   require(GetFileType(original)==FILE_TYPE_DISK&&GetFileInformationByHandle(original,&info)&&!(info.dwFileAttributes&FILE_ATTRIBUTE_DIRECTORY),"borrowed resource must be a regular disk file");
   handle_=ReOpenFile(original,GENERIC_READ,FILE_SHARE_READ|FILE_SHARE_DELETE,FILE_FLAG_OVERLAPPED|FILE_FLAG_RANDOM_ACCESS);
   require(handle_!=INVALID_HANDLE_VALUE,"cannot reopen borrowed file read-only");
   LARGE_INTEGER length{};require(GetFileSizeEx(handle_,&length)&&length.QuadPart>0,"invalid encoded resource length");size_=uint64_t(length.QuadPart);
   event_=CreateEventW(nullptr,TRUE,FALSE,nullptr);require(event_!=nullptr,"cannot create resource read event");
#else
   require(borrowed<=uint64_t(INT32_MAX),"invalid borrowed file descriptor");
   auto original=int(borrowed);struct stat info{};auto flags=fcntl(original,F_GETFL);
   require(flags>=0&&(flags&O_ACCMODE)==O_RDONLY&&fstat(original,&info)==0&&S_ISREG(info.st_mode)&&info.st_size>0,"borrowed resource must be a read-only regular file");
   handle_=fcntl(original,F_DUPFD_CLOEXEC,0);require(handle_>=0,"cannot duplicate borrowed file");size_=uint64_t(info.st_size);
#endif
  }catch(...){close();throw;}
 }
 ~EncodedFile(){close();}
 EncodedFile(const EncodedFile &)=delete;
 EncodedFile &operator=(const EncodedFile &)=delete;
 uint64_t size() const noexcept{return size_;}
 uint64_t position() const noexcept{return position_;}
 size_t readAt(void *output,size_t bytes,uint64_t offset){
  require(output||!bytes,"missing resource read buffer");
  if(offset>=size_||!bytes)return 0;
  auto count=uint32_t(std::min<uint64_t>({uint64_t(bytes),uint64_t(65536),size_-offset}));
#ifdef _WIN32
  OVERLAPPED operation{};operation.Offset=DWORD(offset);operation.OffsetHigh=DWORD(offset>>32);operation.hEvent=event_;
  require(ResetEvent(event_)!=0,"cannot reset resource read event");DWORD read=0;
  if(!ReadFile(handle_,output,count,&read,&operation)){
   auto code=GetLastError();if(code==ERROR_HANDLE_EOF)return 0;
   require(code==ERROR_IO_PENDING,"cannot read borrowed resource");
   if(!GetOverlappedResult(handle_,&operation,&read,TRUE)){require(GetLastError()==ERROR_HANDLE_EOF,"borrowed resource read failed");return 0;}
  }
  return size_t(read);
#else
  ssize_t read;
  do{read=pread(handle_,output,count,off_t(offset));}while(read<0&&errno==EINTR);
  require(read>=0,"cannot read borrowed resource");return size_t(read);
#endif
 }
 size_t read(void *output,size_t bytes){auto count=readAt(output,bytes,position_);position_+=count;return count;}
 bool seek(int64_t offset,unsigned origin) noexcept {
  uint64_t base=origin==0?0:origin==1?position_:size_;
  if(origin>2)return false;
  uint64_t next;
  if(offset<0){auto magnitude=uint64_t(-(offset+1))+1;if(magnitude>base)return false;next=base-magnitude;}
  else{if(uint64_t(offset)>size_-base)return false;next=base+uint64_t(offset);}
  if(next>size_)return false;position_=next;return true;
 }
};
}
#endif
