//! Worker-only ALAC fallback for Windows' incomplete MP4 container support.
//! The caller retains the encoded resource until this decoder is closed.
use std::ffi::{c_char, c_void};
use std::io::{self, Read, Seek, SeekFrom};
use std::panic::{catch_unwind, AssertUnwindSafe};
use symphonia::core::audio::SampleBuffer;
use symphonia::core::codecs::{Decoder, DecoderOptions, CODEC_TYPE_ALAC};
use symphonia::core::errors::Error;
use symphonia::core::formats::{FormatOptions, FormatReader};
use symphonia::core::io::{MediaSource, MediaSourceStream};
use symphonia::core::meta::MetadataOptions;
use symphonia::core::probe::Hint;

type ReadAt = unsafe extern "C" fn(*mut c_void, *mut u8, usize, u64) -> i64;
struct Source {
    user: *mut c_void,
    read: ReadAt,
    size: u64,
    position: u64,
}
// The C++ resource is worker-owned and never called concurrently. Symphonia's
// MediaSource requires Send+Sync but this adapter performs no asynchronous I/O.
unsafe impl Send for Source {}
unsafe impl Sync for Source {}
impl Read for Source {
    fn read(&mut self, out: &mut [u8]) -> io::Result<usize> {
        let count = out
            .len()
            .min(65536)
            .min(self.size.saturating_sub(self.position) as usize);
        let n = unsafe { (self.read)(self.user, out.as_mut_ptr(), count, self.position) };
        if n < 0 || n as usize > count {
            return Err(io::Error::other("Cannot read original ALAC resource"));
        }
        self.position += n as u64;
        Ok(n as usize)
    }
}
impl Seek for Source {
    fn seek(&mut self, from: SeekFrom) -> io::Result<u64> {
        let pos = match from {
            SeekFrom::Start(n) => n as i128,
            SeekFrom::Current(n) => self.position as i128 + n as i128,
            SeekFrom::End(n) => self.size as i128 + n as i128,
        };
        if pos < 0 || pos > self.size as i128 {
            return Err(io::Error::other("Invalid ALAC encoded seek"));
        }
        self.position = pos as u64;
        Ok(self.position)
    }
}
impl MediaSource for Source {
    fn is_seekable(&self) -> bool {
        true
    }
    fn byte_len(&self) -> Option<u64> {
        Some(self.size)
    }
}

struct Alac {
    format: Box<dyn FormatReader>,
    decoder: Box<dyn Decoder>,
    track: u32,
    channels: usize,
    rate: u32,
    expected: Option<u64>,
    decoded: u64,
    pending: Vec<f32>,
    offset: usize,
}
impl Alac {
    fn open(source: Source) -> Result<Self, String> {
        let stream = MediaSourceStream::new(Box::new(source), Default::default());
        let mut hint = Hint::new();
        hint.with_extension("m4a");
        let format = symphonia::default::get_probe()
            .format(
                &hint,
                stream,
                &FormatOptions::default(),
                &MetadataOptions::default(),
            )
            .map_err(|e| e.to_string())?
            .format;
        let track = format
            .tracks()
            .iter()
            .find(|t| t.codec_params.codec == CODEC_TYPE_ALAC)
            .ok_or("M4A does not contain Apple Lossless audio")?;
        let cookie = track
            .codec_params
            .extra_data
            .as_ref()
            .ok_or("Missing ALAC configuration")?;
        if cookie.len() != 24 && cookie.len() != 48 {
            return Err("Invalid ALAC configuration".into());
        }
        let channels = cookie[9] as usize;
        let rate = u32::from_be_bytes(cookie[20..24].try_into().unwrap());
        if !(1..=2).contains(&channels) || !(8000..=192000).contains(&rate) {
            return Err("ALAC requires mono/stereo audio at 8-192 kHz".into());
        }
        let frame_length = u32::from_be_bytes(cookie[..4].try_into().unwrap());
        if !(1..=65536).contains(&frame_length) {
            return Err("Invalid or oversized ALAC frame configuration".into());
        }
        let decoder = symphonia::default::get_codecs()
            .make(&track.codec_params, &DecoderOptions::default())
            .map_err(|e| e.to_string())?;
        let id = track.id;
        let expected = track.codec_params.n_frames.filter(|n| *n > 0);
        Ok(Self {
            format,
            decoder,
            track: id,
            channels,
            rate,
            expected,
            decoded: 0,
            pending: Vec::new(),
            offset: 0,
        })
    }
    fn refill(&mut self) -> Result<bool, String> {
        self.pending.clear();
        self.offset = 0;
        for _ in 0..4096 {
            let packet = match self.format.next_packet() {
                Ok(p) => p,
                Err(Error::IoError(e)) if e.kind() == io::ErrorKind::UnexpectedEof => {
                    if self.expected.is_some_and(|n| n != self.decoded) {
                        return Err("Truncated ALAC audio".into());
                    }
                    return Ok(false);
                }
                Err(e) => return Err(e.to_string()),
            };
            if packet.track_id() != self.track {
                continue;
            }
            let decoded = self.decoder.decode(&packet).map_err(|e| e.to_string())?;
            if decoded.spec().channels.count() != self.channels
                || decoded.spec().rate != self.rate
                || decoded.capacity() > 65536
            {
                return Err("Invalid or oversized ALAC audio block".into());
            }
            if decoded.frames() == 0 {
                continue;
            }
            self.decoded += decoded.frames() as u64;
            if self.decoded > 1u64 << 48 {
                return Err("ALAC source is too long".into());
            }
            let mut block = SampleBuffer::<f32>::new(decoded.capacity() as u64, *decoded.spec());
            block.copy_interleaved_ref(decoded);
            self.pending.extend_from_slice(block.samples());
            return Ok(true);
        }
        Err("ALAC decoder made no audio progress".into())
    }
    fn read(&mut self, out: &mut [f32]) -> Result<usize, String> {
        let mut done = 0;
        while done < out.len() {
            if self.offset == self.pending.len() && !self.refill()? {
                break;
            }
            let n = (out.len() - done).min(self.pending.len() - self.offset);
            out[done..done + n].copy_from_slice(&self.pending[self.offset..self.offset + n]);
            done += n;
            self.offset += n;
        }
        Ok(done / self.channels)
    }
}

fn boundary(error: *mut c_char, size: usize, work: impl FnOnce() -> Result<(), String>) -> i32 {
    let result =
        catch_unwind(AssertUnwindSafe(work)).unwrap_or_else(|_| Err("ALAC decoder failed".into()));
    let (code, text) = match result {
        Ok(()) => (0, String::new()),
        Err(e) => (1, e),
    };
    if !error.is_null() && size != 0 {
        let n = text.len().min(size - 1);
        unsafe {
            std::ptr::copy_nonoverlapping(text.as_ptr(), error.cast(), n);
            *error.add(n) = 0;
        }
    }
    code
}

#[no_mangle]
pub unsafe extern "C" fn riffine_alac_open(
    user: *mut c_void,
    read: ReadAt,
    size: u64,
    result: *mut *mut c_void,
    channels: *mut u32,
    rate: *mut u32,
    error: *mut c_char,
    error_size: usize,
) -> i32 {
    *result = std::ptr::null_mut();
    boundary(error, error_size, || {
        let decoder = Alac::open(Source {
            user,
            read,
            size,
            position: 0,
        })?;
        *channels = decoder.channels as u32;
        *rate = decoder.rate;
        *result = Box::into_raw(Box::new(decoder)).cast();
        Ok(())
    })
}
#[no_mangle]
pub unsafe extern "C" fn riffine_alac_read(
    decoder: *mut c_void,
    output: *mut f32,
    capacity: u32,
    produced: *mut u32,
    error: *mut c_char,
    size: usize,
) -> i32 {
    *produced = 0;
    boundary(error, size, || {
        if decoder.is_null() || output.is_null() || !(1..=8192).contains(&capacity) {
            return Err("Invalid ALAC read buffer".into());
        }
        let decoder = &mut *decoder.cast::<Alac>();
        let out = std::slice::from_raw_parts_mut(output, capacity as usize * decoder.channels);
        *produced = decoder.read(out)? as u32;
        Ok(())
    })
}
#[no_mangle]
pub unsafe extern "C" fn riffine_alac_close(decoder: *mut c_void) {
    if !decoder.is_null() {
        drop(Box::from_raw(decoder.cast::<Alac>()));
    }
}
