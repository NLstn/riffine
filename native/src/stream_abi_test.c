/* Compile and execute as C: opaque API must not require C++ headers/types. */
#include "riffine_stream.h"
#include "riffine_audio.h"
#include "riffine_decode.h"
#include "riffine_scheduled.h"
#include "riffine_preview.h"
#include "riffine_selection.h"
int main(void) {
  riffine_stream_config config={1,1,48000,64,1,0};
  riffine_stream *stream=0;
  char error[256];
  riffine_stream_limits limits;
  if(riffine_stream_create(&config,&stream,error,sizeof(error))) return 1;
  int result=riffine_stream_get_limits(stream,&limits,error,sizeof(error));
  riffine_stream_destroy(stream);
  riffine_scheduled_worker *worker=0;
  riffine_scheduled_info scheduled_info;
  riffine_scheduled_block scheduled_block;
  if(riffine_scheduled_open(0,&worker,error,sizeof(error))!=1 || worker) return 1;
  if(riffine_scheduled_info_get(0,&scheduled_info,error,sizeof(error))!=1) return 1;
  if(riffine_scheduled_reset(0,1,error,sizeof(error))!=1) return 1;
  if(riffine_scheduled_submit(0,0,error,sizeof(error))!=1) return 1;
  if(riffine_scheduled_read(0,0,1,&scheduled_block,error,sizeof(error))!=1 || scheduled_block.frames) return 1;
  if(riffine_scheduled_open_handle(UINT64_MAX,&worker,error,sizeof(error))!=1 || worker) return 1;
  riffine_decoder *decoder=0;
  if(riffine_decoder_open_handle(UINT64_MAX,&decoder,error,sizeof(error))!=1 || decoder) return 1;
  riffine_scheduled_close(0);
  if(riffine_render_handle(UINT64_MAX,0,0,1,1,0,error,sizeof(error))!=1) return 1;
  riffine_preview_source *source=0;
  if(riffine_preview_open(0,0,1,1,0,&source,error,sizeof(error))!=1 || source) return 1;
  if(riffine_preview_open_handle(UINT64_MAX,0,1,1,0,&source,error,sizeof(error))!=1 || source) return 1;
  riffine_preview_report preview_report;
  if(riffine_preview_handle(UINT64_MAX,0,1,1,0,1,&preview_report,error,sizeof(error))!=1 || preview_report.output_frames) return 1;
  riffine_selection *selection=0;
  float input=.2f,output=0;
  uint32_t count=0,needs_input=0;
  if(riffine_selection_create(1,48000,1,2,0,&selection,error,sizeof(error))) return 1;
  if(riffine_selection_set_edge_fade_frames(selection,1,error,sizeof(error))) return 1;
  int selection_result=riffine_selection_push(selection,&input,1,error,sizeof(error));
  selection_result|=riffine_selection_read(selection,&output,1,&count,&needs_input,error,sizeof(error));
  riffine_selection_destroy(selection);
  return result || limits.max_input_frames!=64 || selection_result || count!=1 || needs_input || output!=0;
}
