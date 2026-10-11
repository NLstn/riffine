cmake_minimum_required(VERSION 3.24)
project(riffine_wasm_dsp_probe LANGUAGES C CXX)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
include(FetchContent)
# Same immutable revisions as the production processing layer.
FetchContent_Declare(signalsmith-linear
  GIT_REPOSITORY https://github.com/Signalsmith-Audio/linear.git
  GIT_TAG de55e6a50ffcf6f8f43f649692d94691c7025151
  GIT_SUBMODULES "")
FetchContent_Declare(signalsmith-stretch
  GIT_REPOSITORY https://github.com/Signalsmith-Audio/signalsmith-stretch.git
  GIT_TAG a670068d9aeb64913331d5cc29337b19a457a7df
  GIT_SUBMODULES "")
FetchContent_MakeAvailable(signalsmith-linear signalsmith-stretch)
add_executable(stream-probe stream-probe.c ../../native/src/stream.cpp)
target_include_directories(stream-probe PRIVATE ../../native/include)
target_link_libraries(stream-probe PRIVATE signalsmith-stretch)
if(EMSCRIPTEN)
  set_target_properties(stream-probe PROPERTIES SUFFIX ".mjs")
  target_compile_options(stream-probe PRIVATE -fexceptions)
  target_link_options(stream-probe PRIVATE
    -fexceptions
    "-sDISABLE_EXCEPTION_CATCHING=0"
    "-sMODULARIZE=1" "-sEXPORT_ES6=1"
    "-sENVIRONMENT=web,worker,node"
    "-sEXPORTED_FUNCTIONS=[\"_main\"]"
    "-sINITIAL_MEMORY=33554432" "-sALLOW_MEMORY_GROWTH=0"
    "-sEXIT_RUNTIME=0" "-sEXPORTED_RUNTIME_METHODS=[\"HEAPU8\"]")
endif()

if(NOT EMSCRIPTEN)
 add_executable(selection-probe ../../native/src/selection_test.cpp ../../native/src/selection.cpp ../../native/src/stream.cpp)
 target_include_directories(selection-probe PRIVATE ../../native/include)
 target_link_libraries(selection-probe PRIVATE signalsmith-stretch)
endif()
