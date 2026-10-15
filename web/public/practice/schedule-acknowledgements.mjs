// Bounded adapter routing only: core owns source positions, traversals and loop
// eligibility. IDs stay decimal strings across JS to preserve every uint64 bit.
const maxID = (1n << 64n) - 1n;
function identifier(value) {
 if (typeof value !== 'string' || !/^[1-9][0-9]{0,19}$/.test(value) || BigInt(value) > maxID) throw new Error('Invalid schedule identifier.');
 return BigInt(value);
}
function frame(value) { return Number.isSafeInteger(value) && value >= 0; }
export class ScheduleAcknowledgements {
 constructor() { this.entries = []; this.generation = undefined; this.lastSequence = 0n; this.outputEnd = 0; this.observed = 0; }
 reset(generation) {
  const next = identifier(generation);
  if (this.generation !== undefined && next <= identifier(this.generation)) throw new Error('Schedule generation must advance.');
  this.generation = generation; this.entries = []; this.outputEnd = 0; this.observed = 0;
 }
 accept(schedule) {
  const sequence = identifier(schedule?.sequence);
  if (schedule.generation !== this.generation || sequence <= this.lastSequence || !frame(schedule.outputStart) || !frame(schedule.outputEnd) || schedule.outputStart !== this.outputEnd || schedule.outputEnd <= schedule.outputStart) throw new Error('Invalid ordered output schedule.');
  if (this.entries.length >= (this.entries[0]?.parameterPrefix ? 3 : 2)) throw new Error('Output schedule queue is full.');
  // Copy fields: caller cannot mutate an accepted schedule or acknowledgement.
  this.entries.push({ generation: schedule.generation, sequence: schedule.sequence, outputStart: schedule.outputStart, outputEnd: schedule.outputEnd, acknowledged: schedule.outputStart });
  this.lastSequence = sequence; this.outputEnd = schedule.outputEnd;
 }
 drain(presentedFrames, limit) {
  if (!frame(presentedFrames) || presentedFrames < this.observed || presentedFrames > this.outputEnd || !Number.isInteger(limit) || limit < 1 || limit > 128) throw new Error('Invalid bounded presentation acknowledgement.');
  this.observed = presentedFrames;
  const result = [];
  while (this.entries.length && result.length < limit) {
   const entry = this.entries[0];
   const position = Math.min(presentedFrames, entry.outputEnd);
   if (position <= entry.acknowledged) break;
   result.push({ generation: entry.generation, sequence: entry.sequence, consumedOutputFrame: position });
   entry.acknowledged = position;
   if (position < entry.outputEnd) break;
   this.entries.shift();
  }
  return result;
 }
 replaceTail(sequence, outputFrame, replacement) {
  const current=this.entries[0];
  if(!current || current.sequence!==sequence || !frame(outputFrame) || outputFrame<=current.acknowledged || outputFrame>=current.outputEnd || replacement.generation!==this.generation || replacement.outputStart!==outputFrame || !frame(replacement.outputEnd) || replacement.outputEnd<=outputFrame || identifier(replacement.sequence)<=this.lastSequence) throw new Error('Invalid live parameter boundary.');
  // Undelivered output after this boundary belongs to the new parameters. The
  // prefix keeps its ID so acknowledgements already observed remain ordered.
  current.outputEnd=outputFrame;current.parameterPrefix=true;this.entries=[current];this.outputEnd=outputFrame;
  this.accept(replacement);
 }
}
