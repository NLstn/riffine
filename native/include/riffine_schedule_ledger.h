#ifndef RIFFINE_SCHEDULE_LEDGER_H
#define RIFFINE_SCHEDULE_LEDGER_H
#include "riffine_scheduled.h"
#include <algorithm>
#include <array>
namespace riffine {
struct PresentationAcknowledgement {uint64_t generation,sequence,consumed_output_frame;};
/* Serialized control-worker ledger, at most two submitted schedules. Converts
 * presented PCM ordinals into bounded partial/final acknowledgements with exact
 * uint64 IDs. It contains no source interpolation, loop eligibility or ladder
 * policy. Input MUST come from device presentation mapping, never generated or
 * callback-copied counts. Reset requires output/producer/control quiescence. */
class ScheduleLedger {
 struct Range {riffine_schedule schedule{};uint64_t reported=0;};
 std::array<Range,2> ranges_{};
 unsigned front_=0,count_=0;
 uint64_t generation_=0,last_sequence_=0,next_output_=0,presented_=0;
 bool failed_=false;
public:
 bool reset(uint64_t generation) noexcept {
  if(generation<=generation_)return false;
  generation_=generation;front_=count_=0;next_output_=presented_=0;failed_=false;return true;
 }
 bool canSubmit(const riffine_schedule &s) const noexcept {
  return !failed_&&generation_&&s.generation==generation_&&s.sequence>last_sequence_&&count_<2&&s.output_start==next_output_&&s.output_end>s.output_start&&s.output_end<=uint64_t(INT64_MAX);
 }
 bool submit(const riffine_schedule &s) noexcept {
  if(!canSubmit(s))return false;
  ranges_[(front_+count_)%2]={s,s.output_start};++count_;last_sequence_=s.sequence;next_output_=s.output_end;return true;
 }
 bool acknowledge(uint64_t presented,PresentationAcknowledgement *output,uint32_t limit,uint32_t &produced) noexcept {
  produced=0;
  if(failed_||!output||!limit||limit>128)return false;
  if(presented>next_output_){failed_=true;return false;}
  if(presented<presented_)return true;
  presented_=presented;
  while(count_&&produced<limit){
   auto &range=ranges_[front_];const auto &s=range.schedule;
   auto position=std::min(presented,s.output_end);
   if(position<=range.reported)break;
   output[produced++]={s.generation,s.sequence,position};range.reported=position;
   if(position<s.output_end)break;
   front_=(front_+1)%2;--count_;
  }
  return true;
 }
 unsigned pending() const noexcept{return count_;}
 bool error() const noexcept{return failed_;}
};
}
#endif
