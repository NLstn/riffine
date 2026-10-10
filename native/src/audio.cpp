#include "riffine_audio.h"
#define NOMINMAX
#define MINIAUDIO_IMPLEMENTATION
#include "miniaudio.h"
#include "signalsmith-stretch/signalsmith-stretch.h"
#include <algorithm>
#include <cmath>
#include <cstdio>
#include <filesystem>
#include <stdexcept>
#include <vector>

namespace {
void require(bool ok, const char *message) { if (!ok) throw std::runtime_error(message); }
void error_text(char *buffer, size_t size, const char *message) {
  if (buffer && size) std::snprintf(buffer, size, "%s", message);
}
struct Decoder {
  ma_decoder value{};
  explicit Decoder(const char *path) {
    auto config = ma_decoder_config_init(ma_format_f32, 0, 0);
#ifdef _WIN32
    require(ma_decoder_init_file_w(std::filesystem::u8path(path).c_str(), &config, &value) == MA_SUCCESS, "cannot decode input WAV");
#else
    require(ma_decoder_init_file(path, &config, &value) == MA_SUCCESS, "cannot decode input WAV");
#endif
  }
  ~Decoder() { ma_decoder_uninit(&value); }
};
struct Encoder {
  ma_encoder value{};
  FILE *file=nullptr;
  static ma_result write(ma_encoder *encoder,const void *buffer,size_t bytes,size_t *written) {
    *written=std::fwrite(buffer,1,bytes,static_cast<FILE*>(encoder->pUserData));
    return *written==bytes ? MA_SUCCESS : MA_IO_ERROR;
  }
  static ma_result seek(ma_encoder *encoder,ma_int64 offset,ma_seek_origin origin) {
#ifdef _WIN32
    auto result=_fseeki64(static_cast<FILE*>(encoder->pUserData),offset,origin==ma_seek_origin_start ? SEEK_SET : SEEK_CUR);
#else
    auto result=fseeko(static_cast<FILE*>(encoder->pUserData),offset,origin==ma_seek_origin_start ? SEEK_SET : SEEK_CUR);
#endif
    return result==0 ? MA_SUCCESS : MA_IO_ERROR;
  }
  Encoder(const char *path, unsigned channels, unsigned rate) {
    auto config = ma_encoder_config_init(ma_encoding_format_wav, ma_format_s16, channels, rate);
#ifdef _WIN32
    file=_wfopen(std::filesystem::u8path(path).c_str(),L"wbx");
#else
    file=std::fopen(path,"wbx");
#endif
    require(file!=nullptr,"cannot exclusively create output WAV");
    if(ma_encoder_init(write,seek,file,&config,&value)!=MA_SUCCESS){std::fclose(file);file=nullptr;throw std::runtime_error("cannot initialize WAV encoder");}
  }
  ~Encoder() { if(file){ma_encoder_uninit(&value);std::fclose(file);} }
  void finish() {
    ma_encoder_uninit(&value);
    auto result=std::fclose(file);file=nullptr;
    require(result==0,"cannot finalize output WAV");
  }
};
struct Engine {
  ma_engine value{};
  Engine() { require(ma_engine_init(nullptr, &value) == MA_SUCCESS, "cannot open an audio output device"); }
  ~Engine() { ma_engine_uninit(&value); }
};
struct Sound {
  ma_sound value{};
  Sound(ma_engine *engine, const char *path) {
#ifdef _WIN32
    require(ma_sound_init_from_file_w(engine, std::filesystem::u8path(path).c_str(), MA_SOUND_FLAG_STREAM, nullptr, nullptr, &value) == MA_SUCCESS, "cannot open playback file");
#else
    require(ma_sound_init_from_file(engine, path, MA_SOUND_FLAG_STREAM, nullptr, nullptr, &value) == MA_SUCCESS, "cannot open playback file");
#endif
  }
  ~Sound() { ma_sound_uninit(&value); }
};
}

int riffine_render(const char *input, const char *output, int64_t start, int64_t end,
  double speed, double semitones, char *error, size_t error_size) {
  try {
    require(input && output, "paths are required");
    require(!std::filesystem::exists(std::filesystem::u8path(output)), "output already exists");
    require(std::isfinite(speed) && speed >= .25 && speed <= 2 && std::isfinite(semitones) && semitones >= -12 && semitones <= 12, "invalid speed or pitch");
    Decoder decoder(input);
    auto &d = decoder.value;
    ma_uint64 length = 0;
    require(ma_decoder_get_length_in_pcm_frames(&d, &length) == MA_SUCCESS, "unknown source length");
    require(start >= 0 && end > start && uint64_t(end) <= length, "invalid frame selection");
    require(d.outputChannels >= 1 && d.outputChannels <= 2 && d.outputSampleRate >= 8000 && d.outputSampleRate <= 192000, "unsupported audio format");
    auto channels = d.outputChannels;
    signalsmith::stretch::SignalsmithStretch<float> stretch;
    stretch.presetDefault(channels, d.outputSampleRate);
    stretch.setTransposeSemitones(semitones);
    int64_t selected = end-start;
    // Pad short selections to the algorithm's required preroll, then truncate.
    int64_t padded = std::max<int64_t>(selected, stretch.outputSeekLength(float(speed)) + 2);
    int64_t outputFrames = std::llround(double(selected)/speed);
    int64_t paddedOutput = std::llround(double(padded)/speed);
    require(outputFrames > 0, "selection produces no output frames");
    require((padded+paddedOutput)*channels <= 64*1024*1024, "offline buffer limit (256 MiB) exceeded; select a shorter loop");
    std::vector<std::vector<float>> in(channels, std::vector<float>(size_t(padded), 0));
    std::vector<std::vector<float>> out(channels, std::vector<float>(size_t(paddedOutput), 0));
    require(ma_decoder_seek_to_pcm_frame(&d, start) == MA_SUCCESS, "cannot seek input");
    std::vector<float> block(4096*channels);
    for (int64_t offset=0; offset<selected;) {
      auto count=ma_uint64(std::min<int64_t>(4096,selected-offset)); ma_uint64 read=0;
      auto result=ma_decoder_read_pcm_frames(&d,block.data(),count,&read);
      require((result==MA_SUCCESS || result==MA_AT_END) && read==count,"truncated audio data");
      for(ma_uint64 i=0;i<count;++i) for(unsigned c=0;c<channels;++c) in[c][size_t(offset+i)]=block[size_t(i*channels+c)];
      offset+=int64_t(count);
    }
    require(stretch.exact(in,int(padded),out,int(paddedOutput)), "DSP failed to process selection");
    // A short fade reduces clicks when a rendered selection is repeated.
    auto fade=std::min<int64_t>(std::llround(d.outputSampleRate*.005),outputFrames/2);
    Encoder encoder(output,channels,d.outputSampleRate);
    std::vector<int16_t> pcm(4096*channels);
    for(int64_t offset=0;offset<outputFrames;) {
      auto count=ma_uint64(std::min<int64_t>(4096,outputFrames-offset));
      for(ma_uint64 i=0;i<count;++i) {
        int64_t frame=offset+int64_t(i);
        double gain=fade>0 ? std::min({1.0,double(frame)/fade,double(outputFrames-1-frame)/fade}) : 1;
        for(unsigned c=0;c<channels;++c) {
          double v=out[c][size_t(frame)]*gain;
          require(std::isfinite(v),"non-finite DSP output");
          pcm[size_t(i*channels+c)]=int16_t(std::lround(std::clamp(v,-1.0,1.0)*32767));
        }
      }
      ma_uint64 written=0;
      require(ma_encoder_write_pcm_frames(&encoder.value,pcm.data(),count,&written)==MA_SUCCESS && written==count,"cannot write rendered audio");
      offset+=int64_t(count);
    }
    encoder.finish();
    return 0;
  } catch(const std::exception &e) { error_text(error,error_size,e.what());return 1; }
  catch(...) { error_text(error,error_size,"unexpected native error");return 1; }
}

int riffine_play(const char *input, int loop, char *error, size_t error_size) {
  try {
    require(input,"input path required");Engine engine;Sound sound(&engine.value,input);
    ma_sound_set_looping(&sound.value,loop ? MA_TRUE : MA_FALSE);
    require(ma_sound_start(&sound.value)==MA_SUCCESS,"cannot start playback");
    while(!ma_sound_at_end(&sound.value)) ma_sleep(20);
    return 0;
  } catch(const std::exception &e) {error_text(error,error_size,e.what());return 1;}
  catch(...) {error_text(error,error_size,"unexpected native error");return 1;}
}
