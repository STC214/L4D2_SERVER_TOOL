#define WIN32_LEAN_AND_MEAN
#include <windows.h>

#include <cstdarg>
#include <cstdint>
#include <cstdio>
#include <cstring>

#if !defined(_M_IX86)
#error Build this probe as Win32/x86 with MSVC. L4D2 is a 32-bit process.
#endif

extern "C" IMAGE_DOS_HEADER __ImageBase;

namespace {

struct ProbePoint {
    const char* name;
    DWORD rva;
    DWORD max_hits;
    DWORD hits;
    BYTE original;
    BYTE* address;
    bool armed;
    bool disabled;
};

ProbePoint g_points[] = {
    {"details_consume_candidate", 0x21B80, 4, 0, 0, nullptr, false, false},
    {"consume_GameDetailsServer_ref", 0x21BEE, 14, 0, 0, nullptr, false, false},
    {"consume_Server_adronline_ref", 0x21D94, 14, 0, 0, nullptr, false, false},
    {"consume_Server_connectstring_ref", 0x21ECB, 14, 0, 0, nullptr, false, false},
    {"consume_mgr_update_ref", 0x21FFF, 14, 0, 0, nullptr, false, false},
    {"details_fields_candidate", 0x22750, 20, 0, 0, nullptr, false, false},
    {"fields_Server_name_ref", 0x22812, 20, 0, 0, nullptr, false, false},
    {"fields_Server_adronline_ref", 0x22846, 20, 0, 0, nullptr, false, false},
    {"fields_Server_adrlocal_ref", 0x2285D, 20, 0, 0, nullptr, false, false},
    {"fields_numSlots_ref", 0x228C5, 20, 0, 0, nullptr, false, false},
    {"fields_numPlayers_ref", 0x228FA, 20, 0, 0, nullptr, false, false},
};

HANDLE g_log = INVALID_HANDLE_VALUE;
PVOID g_veh = nullptr;
CRITICAL_SECTION g_log_lock;
bool g_log_lock_ready = false;

ProbePoint* g_single_step_point = nullptr;
BYTE* g_matchmaking_base = nullptr;

void LogRaw(const char* text) {
    if (!g_log_lock_ready || g_log == INVALID_HANDLE_VALUE) {
        return;
    }
    EnterCriticalSection(&g_log_lock);
    DWORD written = 0;
    WriteFile(g_log, text, static_cast<DWORD>(strlen(text)), &written, nullptr);
    LeaveCriticalSection(&g_log_lock);
}

void Logf(const char* fmt, ...) {
    char buffer[4096];
    va_list args;
    va_start(args, fmt);
    _vsnprintf_s(buffer, sizeof(buffer), _TRUNCATE, fmt, args);
    va_end(args);
    LogRaw(buffer);
}

bool IsReadablePointer(const void* p, size_t bytes) {
    if (p == nullptr || bytes == 0) {
        return false;
    }
    MEMORY_BASIC_INFORMATION mbi{};
    if (!VirtualQuery(p, &mbi, sizeof(mbi))) {
        return false;
    }
    if (mbi.State != MEM_COMMIT || (mbi.Protect & PAGE_NOACCESS) || (mbi.Protect & PAGE_GUARD)) {
        return false;
    }
    uintptr_t start = reinterpret_cast<uintptr_t>(p);
    uintptr_t end = start + bytes;
    uintptr_t region_end = reinterpret_cast<uintptr_t>(mbi.BaseAddress) + mbi.RegionSize;
    return end >= start && end <= region_end;
}

void CopyPrintableAscii(char* out, size_t out_size, const void* p, size_t max_scan) {
    if (out_size == 0) {
        return;
    }
    out[0] = 0;
    if (!IsReadablePointer(p, 1)) {
        return;
    }
    const unsigned char* s = static_cast<const unsigned char*>(p);
    size_t n = 0;
    for (; n + 1 < out_size && n < max_scan; ++n) {
        if (!IsReadablePointer(s + n, 1)) {
            break;
        }
        unsigned char c = s[n];
        if (c == 0) {
            break;
        }
        if (c < 0x20 || c > 0x7e) {
            if (n == 0) {
                out[0] = 0;
                return;
            }
            break;
        }
        out[n] = static_cast<char>(c);
    }
    out[n] = 0;
}

void DumpPotentialString(const char* label, DWORD value) {
    char text[256];
    CopyPrintableAscii(text, sizeof(text), reinterpret_cast<const void*>(value), 240);
    if (text[0]) {
        Logf("    %s -> \"%s\"\r\n", label, text);
    }
}

void DumpInlineStrings(const char* label, DWORD value) {
    const unsigned char* base = reinterpret_cast<const unsigned char*>(value);
    if (!IsReadablePointer(base, 1)) {
        return;
    }
    int found = 0;
    for (size_t off = 0; off < 2048 && found < 12; ++off) {
        if (!IsReadablePointer(base + off, 1)) {
            break;
        }
        unsigned char c = base[off];
        if (c < 0x20 || c > 0x7e) {
            continue;
        }
        char text[160];
        size_t n = 0;
        for (; n + 1 < sizeof(text) && off+n < 2048; ++n) {
            if (!IsReadablePointer(base + off + n, 1)) {
                break;
            }
            unsigned char ch = base[off+n];
            if (ch == 0) {
                break;
            }
            if (ch < 0x20 || ch > 0x7e) {
                break;
            }
            text[n] = static_cast<char>(ch);
        }
        text[n] = 0;
        if (n >= 5) {
            Logf("    %s +0x%03IX ascii \"%s\"\r\n", label, off, text);
            found++;
            off += n;
        }
    }
}

void DumpNestedPointerStrings(const char* label, DWORD value) {
    DWORD* words = reinterpret_cast<DWORD*>(value);
    if (!IsReadablePointer(words, sizeof(DWORD))) {
        return;
    }
    int found = 0;
    for (int i = 0; i < 16 && found < 4; ++i) {
        if (!IsReadablePointer(words + i, sizeof(DWORD))) {
            break;
        }
        char text[180];
        CopyPrintableAscii(text, sizeof(text), reinterpret_cast<const void*>(words[i]), 170);
        if (strlen(text) >= 5) {
            Logf("    %s [dword+0x%02X] -> 0x%08lX \"%s\"\r\n", label, i * 4, words[i], text);
            found++;
        }
    }
}

void DumpPointerDeep(const char* label, DWORD value) {
    DumpPotentialString(label, value);
    DumpNestedPointerStrings(label, value);
}

void DumpContext(ProbePoint* point, CONTEXT* ctx) {
#if defined(_M_IX86)
    Logf("\r\n[%lu] hit %s #%lu/%lu rva=0x%08lX eip=0x%08lX ecx=0x%08lX esp=0x%08lX ebp=0x%08lX eax=0x%08lX edx=0x%08lX\r\n",
         GetTickCount(), point->name, point->hits, point->max_hits, point->rva, ctx->Eip, ctx->Ecx, ctx->Esp, ctx->Ebp, ctx->Eax, ctx->Edx);

    DumpPointerDeep("ECX", ctx->Ecx);
    DumpPointerDeep("EAX", ctx->Eax);
    DumpPointerDeep("EDX", ctx->Edx);

    if (g_matchmaking_base) {
        DWORD* details_table_global = reinterpret_cast<DWORD*>(g_matchmaking_base + 0x66104);
        if (IsReadablePointer(details_table_global, sizeof(DWORD))) {
            Logf("    [matchmaking+0x66104] details table first slot ptr = 0x%08lX\r\n", *details_table_global);
        }
        DWORD* details_table = reinterpret_cast<DWORD*>(g_matchmaking_base + 0x54480);
        if (IsReadablePointer(details_table, sizeof(DWORD) * 5)) {
            Logf("    details table slots: +0=0x%08lX +4=0x%08lX +8=0x%08lX +C=0x%08lX +10=0x%08lX\r\n",
                 details_table[0], details_table[1], details_table[2], details_table[3], details_table[4]);
        }
    }

    DWORD* stack = reinterpret_cast<DWORD*>(ctx->Esp);
    if (IsReadablePointer(stack, sizeof(DWORD) * 12)) {
        for (int i = 0; i < 12; ++i) {
            DWORD value = stack[i];
            Logf("    [esp+0x%02X] = 0x%08lX\r\n", i * 4, value);
            char label[32];
            _snprintf_s(label, sizeof(label), _TRUNCATE, "esp+0x%02X", i * 4);
            DumpPointerDeep(label, value);
        }
    }

    DWORD* frame = reinterpret_cast<DWORD*>(ctx->Ebp);
    if (IsReadablePointer(frame, sizeof(DWORD) * 8)) {
        for (int i = 2; i < 8; ++i) {
            DWORD value = frame[i];
            Logf("    [ebp+0x%02X] = 0x%08lX\r\n", i * 4, value);
            char label[32];
            _snprintf_s(label, sizeof(label), _TRUNCATE, "ebp+0x%02X", i * 4);
            DumpPointerDeep(label, value);
        }
    }
#else
    Logf("Unsupported architecture. Build this DLL as Win32/x86.\r\n");
#endif
}

bool ProtectWrite(void* address, const void* data, size_t size) {
    DWORD old_protect = 0;
    if (!VirtualProtect(address, size, PAGE_EXECUTE_READWRITE, &old_protect)) {
        return false;
    }
    memcpy(address, data, size);
    FlushInstructionCache(GetCurrentProcess(), address, size);
    DWORD ignored = 0;
    VirtualProtect(address, size, old_protect, &ignored);
    return true;
}

bool ArmPoint(ProbePoint* point) {
    if (!point->address || point->armed || point->disabled) {
        return false;
    }
    BYTE trap = 0xCC;
    if (!ProtectWrite(point->address, &trap, 1)) {
        return false;
    }
    point->armed = true;
    return true;
}

bool DisarmPoint(ProbePoint* point) {
    if (!point->address || !point->armed) {
        return false;
    }
    if (!ProtectWrite(point->address, &point->original, 1)) {
        return false;
    }
    point->armed = false;
    return true;
}

ProbePoint* FindPointByAddress(void* address) {
    for (auto& point : g_points) {
        if (point.address == address) {
            return &point;
        }
    }
    return nullptr;
}

LONG CALLBACK VehHandler(EXCEPTION_POINTERS* info) {
    if (!info || !info->ExceptionRecord || !info->ContextRecord) {
        return EXCEPTION_CONTINUE_SEARCH;
    }

    DWORD code = info->ExceptionRecord->ExceptionCode;
    CONTEXT* ctx = info->ContextRecord;

    if (code == EXCEPTION_BREAKPOINT) {
        void* hit = info->ExceptionRecord->ExceptionAddress;
        ProbePoint* point = FindPointByAddress(hit);
        if (!point) {
            return EXCEPTION_CONTINUE_SEARCH;
        }
        point->hits++;
        DumpContext(point, ctx);
        DisarmPoint(point);
        if (point->max_hits > 0 && point->hits >= point->max_hits) {
            point->disabled = true;
            Logf("auto-disabled %s after %lu hits\r\n", point->name, point->hits);
        }
#if defined(_M_IX86)
        ctx->Eip = reinterpret_cast<DWORD>(point->address);
        ctx->EFlags |= 0x100; // Trap flag: re-arm after the restored instruction executes.
#endif
        g_single_step_point = point;
        return EXCEPTION_CONTINUE_EXECUTION;
    }

    if (code == EXCEPTION_SINGLE_STEP && g_single_step_point) {
        if (!g_single_step_point->disabled) {
            ArmPoint(g_single_step_point);
        }
        g_single_step_point = nullptr;
        return EXCEPTION_CONTINUE_EXECUTION;
    }

    return EXCEPTION_CONTINUE_SEARCH;
}

DWORD WINAPI WorkerThread(LPVOID) {
    InitializeCriticalSection(&g_log_lock);
    g_log_lock_ready = true;

    char dll_path[MAX_PATH]{};
    GetModuleFileNameA(reinterpret_cast<HMODULE>(&__ImageBase), dll_path, MAX_PATH);
    char* slash = strrchr(dll_path, '\\');
    if (slash) {
        slash[1] = 0;
    } else {
        dll_path[0] = 0;
    }
    char log_path[MAX_PATH]{};
    _snprintf_s(log_path, sizeof(log_path), _TRUNCATE, "%smatchmaking_probe.log", dll_path);

    g_log = CreateFileA(log_path, GENERIC_WRITE, FILE_SHARE_READ, nullptr, CREATE_ALWAYS, FILE_ATTRIBUTE_NORMAL, nullptr);
    Logf("matchmaking_probe loaded. pid=%lu\r\n", GetCurrentProcessId());

    HMODULE matchmaking = nullptr;
    for (int i = 0; i < 200 && !matchmaking; ++i) {
        matchmaking = GetModuleHandleA("matchmaking.dll");
        if (!matchmaking) {
            Sleep(100);
        }
    }
    if (!matchmaking) {
        LogRaw("matchmaking.dll not loaded; probe disabled.\r\n");
        return 0;
    }

    BYTE* base = reinterpret_cast<BYTE*>(matchmaking);
    g_matchmaking_base = base;
    Logf("matchmaking.dll base=0x%p\r\n", base);

    for (auto& point : g_points) {
        point.address = base + point.rva;
        if (!IsReadablePointer(point.address, 1)) {
            Logf("skip %s rva=0x%08lX unreadable\r\n", point.name, point.rva);
            continue;
        }
        point.original = *point.address;
        Logf("point %s rva=0x%08lX addr=0x%p original=0x%02X max_hits=%lu\r\n",
             point.name, point.rva, point.address, point.original, point.max_hits);
    }

    g_veh = AddVectoredExceptionHandler(1, VehHandler);
    if (!g_veh) {
        LogRaw("AddVectoredExceptionHandler failed.\r\n");
        return 0;
    }

    for (auto& point : g_points) {
        if (ArmPoint(&point)) {
            Logf("armed %s\r\n", point.name);
        } else {
            Logf("failed to arm %s\r\n", point.name);
        }
    }

    LogRaw("probe ready.\r\n");
    return 0;
}

} // namespace

BOOL APIENTRY DllMain(HMODULE module, DWORD reason, LPVOID) {
    if (reason == DLL_PROCESS_ATTACH) {
        DisableThreadLibraryCalls(module);
        HANDLE thread = CreateThread(nullptr, 0, WorkerThread, nullptr, 0, nullptr);
        if (thread) {
            CloseHandle(thread);
        }
    }
    return TRUE;
}
