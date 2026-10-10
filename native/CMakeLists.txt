cmake_minimum_required(VERSION 3.24)
project(riffine_audio LANGUAGES C CXX)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
include(FetchContent)
# Declare transitive dependency first to override upstream's mutable tag.
FetchContent_Declare(signalsmith-linear
  GIT_REPOSITORY https://github.com/Signalsmith-Audio/linear.git
  GIT_TAG de55e6a50ffcf6f8f43f649692d94691c7025151
  GIT_SUBMODULES "")
FetchContent_Declare(signalsmith-stretch
  GIT_REPOSITORY https://github.com/Signalsmith-Audio/signalsmith-stretch.git
  GIT_TAG a670068d9aeb64913331d5cc29337b19a457a7df
  GIT_SUBMODULES "")
FetchContent_Declare(miniaudio
  GIT_REPOSITORY https://github.com/mackron/miniaudio.git
  GIT_TAG f40cf03f80cdb7e741d43e53b7e706e8c1394bcf
  GIT_SUBMODULES ""
  SOURCE_SUBDIR riffine-no-upstream-targets)
FetchContent_MakeAvailable(signalsmith-linear signalsmith-stretch miniaudio)
add_library(riffine-dsp STATIC src/audio.cpp src/stream.cpp src/preview_source.cpp src/preview.cpp)
target_include_directories(riffine-dsp PUBLIC include PRIVATE ${miniaudio_SOURCE_DIR})
find_package(Threads REQUIRED)
target_link_libraries(riffine-dsp PRIVATE signalsmith-stretch Threads::Threads)
if(UNIX AND NOT APPLE)
  target_link_libraries(riffine-dsp PRIVATE pthread m dl)
endif()
add_executable(riffine-audio src/main.cpp)
target_link_libraries(riffine-audio PRIVATE riffine-dsp)
install(TARGETS riffine-audio RUNTIME DESTINATION bin)
install(FILES ${signalsmith-stretch_SOURCE_DIR}/LICENSE.txt DESTINATION licenses RENAME signalsmith-stretch.txt)
install(FILES ${signalsmith-linear_SOURCE_DIR}/LICENSE.txt DESTINATION licenses RENAME signalsmith-linear.txt)
install(FILES ${miniaudio_SOURCE_DIR}/LICENSE DESTINATION licenses RENAME miniaudio.txt)
include(CTest)
if(BUILD_TESTING)
  add_executable(riffine-preview-test src/preview_test.cpp)
  target_link_libraries(riffine-preview-test PRIVATE riffine-dsp)
  if(MSVC)
    target_compile_options(riffine-preview-test PRIVATE /utf-8)
  endif()
  add_test(NAME streaming-preview-pipeline COMMAND riffine-preview-test)
  add_executable(riffine-stream-test src/stream_test.cpp)
  target_link_libraries(riffine-stream-test PRIVATE riffine-dsp Threads::Threads)
  add_test(NAME bounded-streaming-dsp COMMAND riffine-stream-test)
  add_test(NAME streaming-long-session COMMAND riffine-stream-test --soak 1800)
  add_executable(riffine-stream-abi-test src/stream_abi_test.c)
  target_link_libraries(riffine-stream-abi-test PRIVATE riffine-dsp)
  set_target_properties(riffine-stream-abi-test PROPERTIES LINKER_LANGUAGE CXX)
  add_test(NAME streaming-c-abi COMMAND riffine-stream-abi-test)
endif()
