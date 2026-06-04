#ifndef _WIN32_WINNT
#define _WIN32_WINNT 0x0601
#endif
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <psapi.h>

#pragma comment(lib, "psapi.lib")

#include <cstdarg>
#include <cstdint>
#include <cstdio>
#include <cstring>

#if !defined(_M_IX86)
#error Build this DLL as Win32/x86 with MSVC. L4D2 is a 32-bit process.
#endif

extern "C" IMAGE_DOS_HEADER __ImageBase;

namespace {

struct Breakpoint {
    const char* name;
    DWORD rva;
    BYTE original;
    BYTE* address;
    bool armed;
};

Breakpoint g_refresh_entry_bp = {"group_refresh_entry", 0x211E0, 0, nullptr, false};
Breakpoint g_table_update_bp = {"table_update_callback", 0x219D0, 0, nullptr, false};
Breakpoint g_details_entry_bp = {"details_consume_entry", 0x21B80, 0, nullptr, false};
Breakpoint g_table_slot2_bp = {"table_slot2_callback", 0x22120, 0, nullptr, false};
Breakpoint g_connectstring_bp = {"connectstring", 0x21ECB, 0, nullptr, false};
Breakpoint g_update_emit_bp = {"update_emit_begin", 0x21FE7, 0, nullptr, false};
Breakpoint g_primary_callback_bp = {"matchmaking_primary_callback", 0x16F08, 0, nullptr, false};
Breakpoint g_dispatch_callback_bp = {"matchmaking_dispatch_callback", 0x16F74, 0, nullptr, false};
Breakpoint g_client_groupserver_bp = {"client_groupserver_candidate", 0x1A0290, 0, nullptr, false};
Breakpoint g_client_row_field_bp = {"client_row_field_candidate", 0x1A0440, 0, nullptr, false};
Breakpoint g_client_row_submit_bp = {"client_row_submit_candidate", 0x1A0630, 0, nullptr, false};
Breakpoint g_client_server_name_bp = {"client_server_name_candidate", 0x1A0A70, 0, nullptr, false};
Breakpoint g_client_server_row_bp = {"client_server_row_candidate", 0x1A16F0, 0, nullptr, false};
Breakpoint g_client_thirdparty_panel_bp = {"client_thirdparty_panel_setup", 0x2EA760, 0, nullptr, false};
Breakpoint g_sb_refresh_row_bp = {"serverbrowser_refresh_row", 0x7504, 0, nullptr, false};
Breakpoint g_sb_single_row_bp = {"serverbrowser_single_row", 0xC7C6, 0, nullptr, false};

HANDLE g_log = INVALID_HANDLE_VALUE;
CRITICAL_SECTION g_log_lock;
bool g_log_lock_ready = false;
PVOID g_veh = nullptr;
BYTE* g_matchmaking_base = nullptr;
BYTE* g_client_base = nullptr;
BYTE* g_serverbrowser_base = nullptr;
DWORD g_skip_thread = 0;
DWORD g_seen = 0;
DWORD g_skipped = 0;
DWORD g_refreshes = 0;
DWORD g_entry_hits = 0;
DWORD g_table_probe_logs = 0;
DWORD g_client_probe_logs = 0;
DWORD g_client_call_probe_logs = 0;
DWORD g_primary_probe_logs = 0;
DWORD g_dispatch_probe_logs = 0;
DWORD g_serverbrowser_probe_logs = 0;
DWORD g_recvfrom_patched = 0;
DWORD g_recvfrom_seen = 0;
DWORD g_recvfrom_dropped = 0;
DWORD g_steam_requests = 0;
DWORD g_steam_response_patched = 0;
DWORD g_steam_responded_seen = 0;
DWORD g_steam_responded_dropped = 0;
DWORD g_steam_details_seen = 0;
DWORD g_steam_details_hidden = 0;
DWORD g_steam_default_name_probe_logs = 0;
DWORD g_steam_disguised_hits = 0;
DWORD g_steam_address_hits = 0;
bool g_early_skip = false;
bool g_neutralize_keywords = true;
bool g_client_skip_name = false;
bool g_dispatch_skip = false;
bool g_client_mark_hidden = false;
bool g_serverbrowser_row_skip = false;
bool g_recvfrom_payload_drop = false;
bool g_steam_serverlist_drop = false;
DWORD g_neutralized = 0;
DWORD g_client_neutralized = 0;

typedef int (__stdcall *RecvFromFn)(UINT_PTR s, char* buf, int len, int flags, void* from, int* fromlen);
typedef void (__stdcall *WSASetLastErrorFn)(int error);
RecvFromFn g_recvfrom_original = nullptr;
WSASetLastErrorFn g_wsa_set_last_error = nullptr;

typedef int (__cdecl *SteamAPIGetHSteamUserFn)();
typedef void* (__cdecl *SteamInternalFindOrCreateUserInterfaceFn)(int user, const char* version);
typedef void* (__thiscall *RequestServerListFn)(void* self, unsigned int appid, void* filters, unsigned int filter_count, void* response);
typedef void* (__thiscall *GetServerDetailsFn)(void* self, void* request, int server);
typedef void (__thiscall *ServerRespondedFn)(void* self, void* request, int server);

void* g_steam_servers = nullptr;
RequestServerListFn g_request_server_list_originals[6]{};
GetServerDetailsFn g_get_server_details_original = nullptr;

struct ResponseVTablePatch {
    void** vtable;
    ServerRespondedFn original;
};

ResponseVTablePatch g_response_vtables[32]{};

char g_filters[64][128]{};
int g_filter_count = 0;
char g_keywords[64][128]{};
int g_keyword_count = 0;
char g_unique_seen[128][64]{};
int g_unique_seen_count = 0;
char g_unique_matches[256][64]{};
int g_unique_match_count = 0;
char g_dll_dir[MAX_PATH]{};

struct DerivedAddressCandidate {
    char host[64];
    int count;
    bool promoted;
};

DerivedAddressCandidate g_derived_candidates[64]{};

struct AutoDerivedPersistJob {
    char host[64];
    char reason[128];
};

struct EntryContext {
    DWORD tid;
    DWORD seq;
    DWORD ecx;
    DWORD ret;
    DWORD arg0;
    bool keyword_block;
    char keyword[128];
};

EntryContext g_entry_contexts[32]{};

struct KeywordLocation {
    const char* keyword;
    DWORD root;
    DWORD address;
    int field_index;
    bool direct;
};

struct StepContext {
    DWORD tid;
    Breakpoint* bp;
};

StepContext g_step_contexts[64]{};

int FilterCapacity() {
    return static_cast<int>(sizeof(g_filters) / sizeof(g_filters[0]));
}

int KeywordCapacity() {
    return static_cast<int>(sizeof(g_keywords) / sizeof(g_keywords[0]));
}

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
    char buffer[1024];
    va_list args;
    va_start(args, fmt);
    _vsnprintf_s(buffer, sizeof(buffer), _TRUNCATE, fmt, args);
    va_end(args);
    LogRaw(buffer);
}

bool IsReadablePointer(const void* p, size_t bytes) {
    if (!p || bytes == 0) {
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

void CopyAscii(char* out, size_t out_size, const void* p, size_t max_scan) {
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

bool ReadDword(DWORD address, DWORD* out) {
    if (!out || !IsReadablePointer(reinterpret_cast<const void*>(address), sizeof(DWORD))) {
        return false;
    }
    *out = *reinterpret_cast<const DWORD*>(address);
    return true;
}

bool WriteByteSafe(DWORD address, unsigned char value) {
    unsigned char* ptr = reinterpret_cast<unsigned char*>(address);
    if (!IsReadablePointer(ptr, sizeof(unsigned char))) {
        return false;
    }
    DWORD old_protect = 0;
    if (!VirtualProtect(ptr, sizeof(unsigned char), PAGE_READWRITE, &old_protect)) {
        return false;
    }
    *ptr = value;
    DWORD ignored = 0;
    VirtualProtect(ptr, sizeof(unsigned char), old_protect, &ignored);
    return true;
}

void Trim(char* s) {
    size_t n = strlen(s);
    while (n > 0 && (s[n - 1] == '\r' || s[n - 1] == '\n' || s[n - 1] == ' ' || s[n - 1] == '\t')) {
        s[--n] = 0;
    }
    char* p = s;
    while (*p == ' ' || *p == '\t') {
        ++p;
    }
    if (p != s) {
        memmove(s, p, strlen(p) + 1);
    }
}

void AddFilter(const char* text) {
    if (!text || !text[0] || g_filter_count >= FilterCapacity()) {
        return;
    }
    for (int i = 0; i < g_filter_count; ++i) {
        if (_stricmp(g_filters[i], text) == 0) {
            return;
        }
    }
    strncpy_s(g_filters[g_filter_count], text, _TRUNCATE);
    g_filter_count++;
}

void AddKeyword(const char* text) {
    if (!text || !text[0] || g_keyword_count >= KeywordCapacity()) {
        return;
    }
    strncpy_s(g_keywords[g_keyword_count], text, _TRUNCATE);
    g_keyword_count++;
}

void LoadFilters(const char* dll_dir) {
    char path[MAX_PATH]{};
    _snprintf_s(path, sizeof(path), _TRUNCATE, "%sblocked_connectstrings.txt", dll_dir);
    FILE* f = nullptr;
    fopen_s(&f, path, "rb");
    if (!f) {
        AddFilter("183.216.53.181");
        AddFilter("118.25.230.120:28888");
        AddFilter("118.25.230.120:33333");
        Logf("filter config not found, using defaults: %s\r\n", path);
    } else {
        char line[256];
        while (fgets(line, sizeof(line), f)) {
            Trim(line);
            if (!line[0] || line[0] == '#') {
                continue;
            }
            AddFilter(line);
        }
        fclose(f);
        Logf("loaded %d filter entries from %s\r\n", g_filter_count, path);
    }

    char learned_path[MAX_PATH]{};
    _snprintf_s(learned_path, sizeof(learned_path), _TRUNCATE, "%slearned_connectstrings.txt", dll_dir);
    FILE* learned = nullptr;
    fopen_s(&learned, learned_path, "rb");
    if (!learned) {
        Logf("learned filter config not found: %s\r\n", learned_path);
    } else {
        int before = g_filter_count;
        char learned_line[256];
        while (fgets(learned_line, sizeof(learned_line), learned)) {
            Trim(learned_line);
            if (!learned_line[0] || learned_line[0] == '#') {
                continue;
            }
            AddFilter(learned_line);
        }
        fclose(learned);
        Logf("loaded %d learned filter entries from %s total=%d\r\n",
             g_filter_count - before, learned_path, g_filter_count);
    }

    char derived_path[MAX_PATH]{};
    _snprintf_s(derived_path, sizeof(derived_path), _TRUNCATE, "%sauto_derived_connectstrings.txt", dll_dir);
    FILE* derived = nullptr;
    fopen_s(&derived, derived_path, "rb");
    if (!derived) {
        Logf("auto-derived filter config not found: %s\r\n", derived_path);
        return;
    }
    int before = g_filter_count;
    char derived_line[256];
    while (fgets(derived_line, sizeof(derived_line), derived)) {
        Trim(derived_line);
        if (!derived_line[0] || derived_line[0] == '#') {
            continue;
        }
        AddFilter(derived_line);
    }
    fclose(derived);
    Logf("loaded %d auto-derived filter entries from %s total=%d\r\n",
         g_filter_count - before, derived_path, g_filter_count);
}

void LoadKeywords(const char* dll_dir) {
    char path[MAX_PATH]{};
    _snprintf_s(path, sizeof(path), _TRUNCATE, "%sblocked_keywords.txt", dll_dir);
    FILE* f = nullptr;
    fopen_s(&f, path, "rb");
    if (!f) {
        Logf("keyword config not found: %s\r\n", path);
        return;
    }
    char line[256];
    while (fgets(line, sizeof(line), f)) {
        Trim(line);
        if (!line[0] || line[0] == '#') {
            continue;
        }
        AddKeyword(line);
    }
    fclose(f);
    Logf("loaded %d keyword entries from %s\r\n", g_keyword_count, path);
}

void LoadMode(const char* dll_dir) {
    char path[MAX_PATH]{};
    _snprintf_s(path, sizeof(path), _TRUNCATE, "%srow_filter_mode.txt", dll_dir);
    FILE* f = nullptr;
    fopen_s(&f, path, "rb");
    if (!f) {
        LogRaw("mode=neutralize_keyword_then_late_skip (default)\r\n");
        return;
    }
    char line[64]{};
    if (fgets(line, sizeof(line), f)) {
        Trim(line);
        if (_stricmp(line, "early") == 0 || _stricmp(line, "early_consume_skip") == 0) {
            g_early_skip = true;
            g_neutralize_keywords = false;
            g_client_skip_name = false;
            g_dispatch_skip = false;
            g_client_mark_hidden = false;
            g_serverbrowser_row_skip = false;
            g_recvfrom_payload_drop = false;
            g_steam_serverlist_drop = false;
            LogRaw("mode=early_consume_skip (experimental, crash-prone)\r\n");
        } else if (_stricmp(line, "late") == 0 || _stricmp(line, "late_update_skip") == 0) {
            g_early_skip = false;
            g_neutralize_keywords = false;
            g_client_skip_name = false;
            g_dispatch_skip = false;
            g_client_mark_hidden = false;
            g_serverbrowser_row_skip = false;
            g_recvfrom_payload_drop = false;
            g_steam_serverlist_drop = false;
            LogRaw("mode=late_update_skip\r\n");
        } else if (_stricmp(line, "ui_skip_name") == 0 || _stricmp(line, "client_skip_name") == 0) {
            g_early_skip = false;
            g_neutralize_keywords = true;
            g_client_skip_name = true;
            g_dispatch_skip = false;
            g_client_mark_hidden = false;
            g_serverbrowser_row_skip = false;
            g_recvfrom_payload_drop = false;
            g_steam_serverlist_drop = false;
            LogRaw("mode=ui_skip_name (experimental)\r\n");
        } else if (_stricmp(line, "ui_skip_name_only") == 0 || _stricmp(line, "client_skip_name_only") == 0) {
            g_early_skip = false;
            g_neutralize_keywords = false;
            g_client_skip_name = true;
            g_dispatch_skip = false;
            g_client_mark_hidden = false;
            g_serverbrowser_row_skip = false;
            g_recvfrom_payload_drop = false;
            g_steam_serverlist_drop = false;
            LogRaw("mode=ui_skip_name_only (experimental, no neutralization)\r\n");
        } else if (_stricmp(line, "dispatch_skip_mark_hidden") == 0 || _stricmp(line, "dispatch_skip_client_mark_hidden") == 0) {
            g_early_skip = false;
            g_neutralize_keywords = false;
            g_client_skip_name = true;
            g_dispatch_skip = true;
            g_client_mark_hidden = true;
            g_serverbrowser_row_skip = true;
            g_recvfrom_payload_drop = false;
            g_steam_serverlist_drop = true;
            LogRaw("mode=dispatch_skip_mark_hidden (experimental, dispatch skip plus UI state mark plus serverbrowser row skip plus Steam ServerResponded drop)\r\n");
        } else if (_stricmp(line, "dispatch_skip_client_name") == 0 || _stricmp(line, "dispatch_skip_ui_name") == 0) {
            g_early_skip = false;
            g_neutralize_keywords = false;
            g_client_skip_name = true;
            g_dispatch_skip = true;
            g_client_mark_hidden = false;
            g_serverbrowser_row_skip = true;
            g_recvfrom_payload_drop = false;
            g_steam_serverlist_drop = true;
            LogRaw("mode=dispatch_skip_client_name (experimental, dispatch skip plus UI name return plus serverbrowser row skip plus Steam ServerResponded drop)\r\n");
        } else if (_stricmp(line, "serverbrowser_row_skip") == 0 || _stricmp(line, "serverbrowser_skip") == 0) {
            g_early_skip = false;
            g_neutralize_keywords = false;
            g_client_skip_name = false;
            g_dispatch_skip = false;
            g_client_mark_hidden = false;
            g_serverbrowser_row_skip = true;
            g_recvfrom_payload_drop = false;
            g_steam_serverlist_drop = false;
            LogRaw("mode=serverbrowser_row_skip (experimental, list row creation only)\r\n");
        } else if (_stricmp(line, "payload_drop") == 0 || _stricmp(line, "recvfrom_payload_drop") == 0) {
            g_early_skip = false;
            g_neutralize_keywords = false;
            g_client_skip_name = false;
            g_dispatch_skip = false;
            g_client_mark_hidden = false;
            g_serverbrowser_row_skip = false;
            g_recvfrom_payload_drop = true;
            g_steam_serverlist_drop = false;
            LogRaw("mode=recvfrom_payload_drop (experimental, UDP payload drop only)\r\n");
        } else if (_stricmp(line, "steam_serverlist_drop") == 0 || _stricmp(line, "steam_callback_drop") == 0) {
            g_early_skip = false;
            g_neutralize_keywords = false;
            g_client_skip_name = false;
            g_dispatch_skip = false;
            g_client_mark_hidden = false;
            g_serverbrowser_row_skip = false;
            g_recvfrom_payload_drop = false;
            g_steam_serverlist_drop = true;
            LogRaw("mode=steam_serverlist_drop (experimental, Steam ServerResponded only)\r\n");
        } else if (_stricmp(line, "dispatch_skip") == 0 || _stricmp(line, "dispatch_skip_only") == 0) {
            g_early_skip = false;
            g_neutralize_keywords = false;
            g_client_skip_name = false;
            g_dispatch_skip = true;
            g_client_mark_hidden = false;
            g_serverbrowser_row_skip = true;
            g_recvfrom_payload_drop = false;
            g_steam_serverlist_drop = true;
            LogRaw("mode=dispatch_skip_only (experimental, no neutralization)\r\n");
        } else {
            g_early_skip = false;
            g_neutralize_keywords = true;
            g_client_skip_name = false;
            g_dispatch_skip = false;
            g_client_mark_hidden = false;
            g_serverbrowser_row_skip = false;
            g_recvfrom_payload_drop = false;
            g_steam_serverlist_drop = false;
            LogRaw("mode=neutralize_keyword_then_late_skip\r\n");
        }
    }
    fclose(f);
}

bool ShouldBlock(const char* connect) {
    if (!connect || !connect[0]) {
        return false;
    }
    for (int i = 0; i < g_filter_count; ++i) {
        if (strstr(connect, g_filters[i])) {
            return true;
        }
    }
    return false;
}

bool IsHighValueAddressForLog(const char* address) {
    if (!address || !address[0]) {
        return false;
    }
    return strstr(address, "110.42.9.24") ||
           strstr(address, "24.9.42.110") ||
           strstr(address, "61.147.247.31") ||
           strstr(address, "31.247.147.61");
}

unsigned char LowerAscii(unsigned char c) {
    if (c >= 'A' && c <= 'Z') {
        return static_cast<unsigned char>(c + ('a' - 'A'));
    }
    return c;
}

bool EqualsAsciiNoCase(const unsigned char* haystack, const char* needle, size_t needle_len) {
    for (size_t i = 0; i < needle_len; ++i) {
        if (LowerAscii(haystack[i]) != LowerAscii(static_cast<unsigned char>(needle[i]))) {
            return false;
        }
    }
    return true;
}

const char* FindKeywordInBytes(const unsigned char* data, size_t size) {
    if (!data || size == 0 || g_keyword_count == 0) {
        return nullptr;
    }
    for (int k = 0; k < g_keyword_count; ++k) {
        const char* keyword = g_keywords[k];
        size_t needle_len = strlen(keyword);
        if (needle_len == 0 || needle_len > size) {
            continue;
        }
        for (size_t i = 0; i + needle_len <= size; ++i) {
            if (EqualsAsciiNoCase(data + i, keyword, needle_len)) {
                return keyword;
            }
        }
    }
    return nullptr;
}

const char* FindKeywordInRange(DWORD address, size_t max_bytes) {
    if (!address || !max_bytes) {
        return nullptr;
    }
    MEMORY_BASIC_INFORMATION mbi{};
    const unsigned char* p = reinterpret_cast<const unsigned char*>(address);
    if (!VirtualQuery(p, &mbi, sizeof(mbi))) {
        return nullptr;
    }
    if (mbi.State != MEM_COMMIT || (mbi.Protect & PAGE_NOACCESS) || (mbi.Protect & PAGE_GUARD)) {
        return nullptr;
    }
    uintptr_t start = reinterpret_cast<uintptr_t>(p);
    uintptr_t region_end = reinterpret_cast<uintptr_t>(mbi.BaseAddress) + mbi.RegionSize;
    if (start >= region_end) {
        return nullptr;
    }
    size_t available = static_cast<size_t>(region_end - start);
    size_t n = available < max_bytes ? available : max_bytes;
    return FindKeywordInBytes(p, n);
}

bool IsMutableRange(DWORD address, size_t max_bytes, unsigned char** data, size_t* size) {
    if (!address || !max_bytes || !data || !size) {
        return false;
    }
    MEMORY_BASIC_INFORMATION mbi{};
    unsigned char* p = reinterpret_cast<unsigned char*>(address);
    if (!VirtualQuery(p, &mbi, sizeof(mbi))) {
        return false;
    }
    if (mbi.State != MEM_COMMIT || mbi.Type == MEM_IMAGE ||
        (mbi.Protect & PAGE_NOACCESS) || (mbi.Protect & PAGE_GUARD)) {
        return false;
    }
    uintptr_t start = reinterpret_cast<uintptr_t>(p);
    uintptr_t region_end = reinterpret_cast<uintptr_t>(mbi.BaseAddress) + mbi.RegionSize;
    if (start >= region_end) {
        return false;
    }
    size_t available = static_cast<size_t>(region_end - start);
    *size = available < max_bytes ? available : max_bytes;
    *data = p;
    return *size > 0;
}

int NeutralizeKeywordsInRange(DWORD address, size_t max_bytes) {
    unsigned char* data = nullptr;
    size_t size = 0;
    if (!IsMutableRange(address, max_bytes, &data, &size)) {
        return 0;
    }

    int replacements = 0;
    for (int k = 0; k < g_keyword_count; ++k) {
        const char* keyword = g_keywords[k];
        size_t needle_len = strlen(keyword);
        if (needle_len == 0 || needle_len > size) {
            continue;
        }
        for (size_t i = 0; i + needle_len <= size; ++i) {
            if (!EqualsAsciiNoCase(data + i, keyword, needle_len)) {
                continue;
            }
            DWORD old_protect = 0;
            if (!VirtualProtect(data + i, needle_len, PAGE_READWRITE, &old_protect)) {
                continue;
            }
            memset(data + i, '_', needle_len);
            DWORD ignored = 0;
            VirtualProtect(data + i, needle_len, old_protect, &ignored);
            replacements++;
        }
    }
    return replacements;
}

int NeutralizeKeywordsNearObject(DWORD root) {
    int replacements = NeutralizeKeywordsInRange(root, 1024);
    if (!IsReadablePointer(reinterpret_cast<const void*>(root), 128 * sizeof(DWORD))) {
        return replacements;
    }
    const DWORD* fields = reinterpret_cast<const DWORD*>(root);
    for (int i = 0; i < 128; ++i) {
        replacements += NeutralizeKeywordsInRange(fields[i], 2048);
    }
    return replacements;
}

bool IsHotArg0Field(int index) {
    const int hot_fields[] = {8, 13, 25, 37, 49, 61, 73, 97, 109, 121};
    for (int i = 0; i < static_cast<int>(sizeof(hot_fields) / sizeof(hot_fields[0])); ++i) {
        if (hot_fields[i] == index) {
            return true;
        }
    }
    return false;
}

void AppendDeepSummary(char* summary, size_t summary_size, int field, int subfield, int replacements) {
    if (!summary || summary_size == 0 || replacements <= 0) {
        return;
    }
    size_t used = strlen(summary);
    if (used + 24 >= summary_size) {
        return;
    }
    sprintf_s(summary + used, summary_size - used, "f%d.s%d=%d;", field, subfield, replacements);
}

int NeutralizeHotArg0SecondLevel(DWORD root, char* summary, size_t summary_size) {
    if (summary && summary_size > 0) {
        summary[0] = 0;
    }
    if (!IsReadablePointer(reinterpret_cast<const void*>(root), 128 * sizeof(DWORD))) {
        return 0;
    }

    int replacements = 0;
    const DWORD* fields = reinterpret_cast<const DWORD*>(root);
    for (int field = 0; field < 128; ++field) {
        if (!IsHotArg0Field(field)) {
            continue;
        }
        DWORD hot = fields[field];
        if (!IsReadablePointer(reinterpret_cast<const void*>(hot), 32 * sizeof(DWORD))) {
            continue;
        }
        const DWORD* subfields = reinterpret_cast<const DWORD*>(hot);
        for (int subfield = 0; subfield < 32; ++subfield) {
            int changed = NeutralizeKeywordsInRange(subfields[subfield], 1024);
            if (changed > 0) {
                replacements += changed;
                AppendDeepSummary(summary, summary_size, field, subfield, changed);
            }
        }
    }
    return replacements;
}

const char* FindKeywordNearObject(DWORD root) {
    const char* hit = FindKeywordInRange(root, 512);
    if (hit) {
        return hit;
    }
    if (!IsReadablePointer(reinterpret_cast<const void*>(root), 64 * sizeof(DWORD))) {
        return nullptr;
    }
    const DWORD* fields = reinterpret_cast<const DWORD*>(root);
    for (int i = 0; i < 64; ++i) {
        DWORD candidate = fields[i];
        hit = FindKeywordInRange(candidate, 1024);
        if (hit) {
            return hit;
        }
    }
    return nullptr;
}

bool FindKeywordLocationNearObject(DWORD root, size_t root_bytes, int field_count, size_t field_bytes, KeywordLocation* location) {
    if (!location) {
        return false;
    }
    location->keyword = nullptr;
    location->root = root;
    location->address = 0;
    location->field_index = -1;
    location->direct = false;

    const char* hit = FindKeywordInRange(root, root_bytes);
    if (hit) {
        location->keyword = hit;
        location->address = root;
        location->direct = true;
        return true;
    }
    if (!IsReadablePointer(reinterpret_cast<const void*>(root), field_count * sizeof(DWORD))) {
        return false;
    }
    const DWORD* fields = reinterpret_cast<const DWORD*>(root);
    for (int i = 0; i < field_count; ++i) {
        DWORD candidate = fields[i];
        hit = FindKeywordInRange(candidate, field_bytes);
        if (hit) {
            location->keyword = hit;
            location->address = candidate;
            location->field_index = i;
            return true;
        }
    }
    return false;
}

const char* FindKeywordInServerBrowserEntry(DWORD root, char* summary, size_t summary_size) {
    if (summary && summary_size > 0) {
        summary[0] = 0;
    }
    if (!root) {
        return nullptr;
    }

    struct Field {
        DWORD offset;
        size_t bytes;
        const char* name;
    };
    const Field fields[] = {
        {0x0E, 96, "addr_or_map_a"},
        {0x2E, 96, "addr_or_map_b"},
        {0x4E, 192, "game_desc"},
        {0xAC, 256, "name"},
        {0xEC, 512, "tags"},
    };

    for (int i = 0; i < static_cast<int>(sizeof(fields) / sizeof(fields[0])); ++i) {
        const Field& field = fields[i];
        const char* hit = FindKeywordInRange(root + field.offset, field.bytes);
        if (hit) {
            if (summary && summary_size > 0) {
                _snprintf_s(summary, summary_size, _TRUNCATE, "%s+0x%lX", field.name, field.offset);
            }
            return hit;
        }
    }
    return nullptr;
}

bool ContainsBytes(const unsigned char* data, size_t size, const char* text) {
    if (!data || !text) {
        return false;
    }
    size_t text_len = strlen(text);
    if (text_len == 0 || text_len > size) {
        return false;
    }
    for (size_t i = 0; i + text_len <= size; ++i) {
        if (memcmp(data + i, text, text_len) == 0) {
            return true;
        }
    }
    return false;
}

bool ContainsAsciiNoCase(const char* haystack, const char* needle) {
    if (!haystack || !needle || !needle[0]) {
        return false;
    }
    size_t needle_len = strlen(needle);
    size_t haystack_len = strlen(haystack);
    if (needle_len > haystack_len) {
        return false;
    }
    const unsigned char* h = reinterpret_cast<const unsigned char*>(haystack);
    for (size_t i = 0; i + needle_len <= haystack_len; ++i) {
        if (EqualsAsciiNoCase(h + i, needle, needle_len)) {
            return true;
        }
    }
    return false;
}

bool EqualsAsciiStringNoCase(const char* left, const char* right) {
    if (!left || !right) {
        return false;
    }
    size_t left_len = strlen(left);
    size_t right_len = strlen(right);
    return left_len == right_len && EqualsAsciiNoCase(reinterpret_cast<const unsigned char*>(left), right, right_len);
}

bool IsOfficialMapCodeText(const char* text) {
    if (!text || !text[0]) {
        return false;
    }
    return
        ContainsAsciiNoCase(text, "c1m1_hotel") ||
        ContainsAsciiNoCase(text, "c1m2_streets") ||
        ContainsAsciiNoCase(text, "c1m3_mall") ||
        ContainsAsciiNoCase(text, "c1m4_atrium") ||
        ContainsAsciiNoCase(text, "c2m1_highway") ||
        ContainsAsciiNoCase(text, "c2m2_fairgrounds") ||
        ContainsAsciiNoCase(text, "c2m3_coaster") ||
        ContainsAsciiNoCase(text, "c2m4_barns") ||
        ContainsAsciiNoCase(text, "c2m5_concert") ||
        ContainsAsciiNoCase(text, "c3m1_plankcountry") ||
        ContainsAsciiNoCase(text, "c3m2_swamp") ||
        ContainsAsciiNoCase(text, "c3m3_shantytown") ||
        ContainsAsciiNoCase(text, "c3m4_plantation") ||
        ContainsAsciiNoCase(text, "c4m1_milltown_a") ||
        ContainsAsciiNoCase(text, "c4m2_sugarmill_a") ||
        ContainsAsciiNoCase(text, "c4m3_sugarmill_b") ||
        ContainsAsciiNoCase(text, "c4m4_milltown_b") ||
        ContainsAsciiNoCase(text, "c4m5_milltown_escape") ||
        ContainsAsciiNoCase(text, "c5m1_waterfront_sndscape") ||
        ContainsAsciiNoCase(text, "c5m1_waterfront") ||
        ContainsAsciiNoCase(text, "c5m2_park") ||
        ContainsAsciiNoCase(text, "c5m3_cemetery") ||
        ContainsAsciiNoCase(text, "c5m4_quarter") ||
        ContainsAsciiNoCase(text, "c5m5_bridge") ||
        ContainsAsciiNoCase(text, "c6m1_riverbank") ||
        ContainsAsciiNoCase(text, "c6m2_bedlam") ||
        ContainsAsciiNoCase(text, "c6m3_port") ||
        ContainsAsciiNoCase(text, "c7m1_docks") ||
        ContainsAsciiNoCase(text, "c7m2_barge") ||
        ContainsAsciiNoCase(text, "c7m3_port") ||
        ContainsAsciiNoCase(text, "c8m1_apartment") ||
        ContainsAsciiNoCase(text, "c8m2_subway") ||
        ContainsAsciiNoCase(text, "c8m3_sewers") ||
        ContainsAsciiNoCase(text, "c8m4_interior") ||
        ContainsAsciiNoCase(text, "c8m5_rooftop") ||
        ContainsAsciiNoCase(text, "c9m1_alleys") ||
        ContainsAsciiNoCase(text, "c9m2_lots") ||
        ContainsAsciiNoCase(text, "c10m1_caves") ||
        ContainsAsciiNoCase(text, "c10m2_drainage") ||
        ContainsAsciiNoCase(text, "c10m3_ranchhouse") ||
        ContainsAsciiNoCase(text, "c10m4_mainstreet") ||
        ContainsAsciiNoCase(text, "c10m5_houseboat") ||
        ContainsAsciiNoCase(text, "c11m1_greenhouse") ||
        ContainsAsciiNoCase(text, "c11m2_offices") ||
        ContainsAsciiNoCase(text, "c11m3_garage") ||
        ContainsAsciiNoCase(text, "c11m4_terminal") ||
        ContainsAsciiNoCase(text, "c11m5_runway") ||
        ContainsAsciiNoCase(text, "c12m1_hilltop") ||
        ContainsAsciiNoCase(text, "c12m2_traintunnel") ||
        ContainsAsciiNoCase(text, "c12m3_bridge") ||
        ContainsAsciiNoCase(text, "c12m4_barn") ||
        ContainsAsciiNoCase(text, "c12m5_cornfield") ||
        ContainsAsciiNoCase(text, "c13m1_alpinecreek") ||
        ContainsAsciiNoCase(text, "c13m2_southpinestream") ||
        ContainsAsciiNoCase(text, "c13m3_memorialbridge") ||
        ContainsAsciiNoCase(text, "c13m4_cutthroatcreek") ||
        ContainsAsciiNoCase(text, "c14m1_junkyard") ||
        ContainsAsciiNoCase(text, "c14m2_lighthouse");
}

const char* FindOfficialMapCodeInBytes(const unsigned char* data, size_t size) {
    static const char* maps[] = {
        "c1m1_hotel", "c1m2_streets", "c1m3_mall", "c1m4_atrium",
        "c2m1_highway", "c2m2_fairgrounds", "c2m3_coaster", "c2m4_barns", "c2m5_concert",
        "c3m1_plankcountry", "c3m2_swamp", "c3m3_shantytown", "c3m4_plantation",
        "c4m1_milltown_a", "c4m2_sugarmill_a", "c4m3_sugarmill_b", "c4m4_milltown_b", "c4m5_milltown_escape",
        "c5m1_waterfront_sndscape", "c5m1_waterfront", "c5m2_park", "c5m3_cemetery", "c5m4_quarter", "c5m5_bridge",
        "c6m1_riverbank", "c6m2_bedlam", "c6m3_port",
        "c7m1_docks", "c7m2_barge", "c7m3_port",
        "c8m1_apartment", "c8m2_subway", "c8m3_sewers", "c8m4_interior", "c8m5_rooftop",
        "c9m1_alleys", "c9m2_lots",
        "c10m1_caves", "c10m2_drainage", "c10m3_ranchhouse", "c10m4_mainstreet", "c10m5_houseboat",
        "c11m1_greenhouse", "c11m2_offices", "c11m3_garage", "c11m4_terminal", "c11m5_runway",
        "c12m1_hilltop", "c12m2_traintunnel", "c12m3_bridge", "c12m4_barn", "c12m5_cornfield",
        "c13m1_alpinecreek", "c13m2_southpinestream", "c13m3_memorialbridge", "c13m4_cutthroatcreek",
        "c14m1_junkyard", "c14m2_lighthouse",
    };
    if (!data || size == 0) {
        return nullptr;
    }
    for (int m = 0; m < static_cast<int>(sizeof(maps) / sizeof(maps[0])); ++m) {
        const char* map = maps[m];
        size_t map_len = strlen(map);
        if (map_len > size) {
            continue;
        }
        for (size_t i = 0; i + map_len <= size; ++i) {
            if (EqualsAsciiNoCase(data + i, map, map_len)) {
                return map;
            }
        }
    }
    return nullptr;
}

const char* FindOfficialMapCodeInRange(DWORD address, size_t max_bytes) {
    if (!address || !max_bytes) {
        return nullptr;
    }
    MEMORY_BASIC_INFORMATION mbi{};
    const unsigned char* p = reinterpret_cast<const unsigned char*>(address);
    if (!VirtualQuery(p, &mbi, sizeof(mbi))) {
        return nullptr;
    }
    if (mbi.State != MEM_COMMIT || (mbi.Protect & PAGE_NOACCESS) || (mbi.Protect & PAGE_GUARD)) {
        return nullptr;
    }
    uintptr_t start = reinterpret_cast<uintptr_t>(p);
    uintptr_t region_end = reinterpret_cast<uintptr_t>(mbi.BaseAddress) + mbi.RegionSize;
    if (start >= region_end) {
        return nullptr;
    }
    size_t available = static_cast<size_t>(region_end - start);
    size_t n = available < max_bytes ? available : max_bytes;
    return FindOfficialMapCodeInBytes(p, n);
}

bool CopyOfficialMapFromGameServerItem(DWORD root, char* map, size_t map_size) {
    if (map && map_size > 0) {
        map[0] = 0;
    }
    char item_map[64]{};
    CopyAscii(item_map, sizeof(item_map), reinterpret_cast<const void*>(root + 0x2E), 60);
    if (!IsOfficialMapCodeText(item_map)) {
        return false;
    }
    if (map && map_size > 0) {
        strncpy_s(map, map_size, item_map, _TRUNCATE);
    }
    return true;
}

bool FormatSteamItemAddress(DWORD root, char* out, size_t out_size) {
    if (out && out_size > 0) {
        out[0] = 0;
    }
    if (!out || out_size == 0 || !IsReadablePointer(reinterpret_cast<const void*>(root), 8)) {
        return false;
    }

    const unsigned char* bytes = reinterpret_cast<const unsigned char*>(root + 4);
    unsigned short conn_port = *reinterpret_cast<const unsigned short*>(root + 0);
    unsigned short query_port = *reinterpret_cast<const unsigned short*>(root + 2);
    if ((bytes[0] == 0 && bytes[1] == 0 && bytes[2] == 0 && bytes[3] == 0) ||
        (bytes[0] == 0xFF && bytes[1] == 0xFF && bytes[2] == 0xFF && bytes[3] == 0xFF)) {
        return false;
    }

    _snprintf_s(out, out_size, _TRUNCATE,
                "%u.%u.%u.%u:%u/%u %u.%u.%u.%u:%u/%u",
                bytes[0], bytes[1], bytes[2], bytes[3], conn_port, query_port,
                bytes[3], bytes[2], bytes[1], bytes[0], conn_port, query_port);
    return true;
}

bool FormatSteamItemCanonicalHost(DWORD root, char* out, size_t out_size) {
    if (out && out_size > 0) {
        out[0] = 0;
    }
    if (!out || out_size == 0 || !IsReadablePointer(reinterpret_cast<const void*>(root), 8)) {
        return false;
    }
    const unsigned char* bytes = reinterpret_cast<const unsigned char*>(root + 4);
    if ((bytes[0] == 0 && bytes[1] == 0 && bytes[2] == 0 && bytes[3] == 0) ||
        (bytes[0] == 0xFF && bytes[1] == 0xFF && bytes[2] == 0xFF && bytes[3] == 0xFF)) {
        return false;
    }
    _snprintf_s(out, out_size, _TRUNCATE, "%u.%u.%u.%u",
                bytes[3], bytes[2], bytes[1], bytes[0]);
    return true;
}

bool IsAutoDerivedConfigLine(const char* line) {
    if (!line || !line[0] || line[0] == '#') {
        return false;
    }
    return true;
}

bool ReadAutoDerivedHosts(char hosts[][64], int* count, int capacity) {
    if (count) {
        *count = 0;
    }
    if (!hosts || !count || capacity <= 0 || !g_dll_dir[0]) {
        return false;
    }
    char path[MAX_PATH]{};
    _snprintf_s(path, sizeof(path), _TRUNCATE, "%sauto_derived_connectstrings.txt", g_dll_dir);
    FILE* f = nullptr;
    fopen_s(&f, path, "rb");
    if (!f) {
        return true;
    }
    char line[256];
    while (fgets(line, sizeof(line), f)) {
        Trim(line);
        if (!IsAutoDerivedConfigLine(line)) {
            continue;
        }
        bool exists = false;
        for (int i = 0; i < *count; ++i) {
            if (_stricmp(hosts[i], line) == 0) {
                exists = true;
                break;
            }
        }
        if (!exists && *count < capacity) {
            strncpy_s(hosts[*count], 64, line, _TRUNCATE);
            (*count)++;
        }
    }
    fclose(f);
    return true;
}

bool WriteAutoDerivedHostsAtomic(char hosts[][64], int count) {
    if (!hosts || count < 0 || !g_dll_dir[0]) {
        return false;
    }
    char path[MAX_PATH]{};
    char tmp_path[MAX_PATH]{};
    _snprintf_s(path, sizeof(path), _TRUNCATE, "%sauto_derived_connectstrings.txt", g_dll_dir);
    _snprintf_s(tmp_path, sizeof(tmp_path), _TRUNCATE, "%sauto_derived_connectstrings.tmp", g_dll_dir);

    FILE* f = nullptr;
    fopen_s(&f, tmp_path, "wb");
    if (!f) {
        Logf("auto-derived write failed tmp=%s\r\n", tmp_path);
        return false;
    }
    fprintf(f, "# Auto-derived from Steam group-server disguised rows.\r\n");
    fprintf(f, "# Rewritten only when the derived host set changes.\r\n\r\n");
    for (int i = 0; i < count; ++i) {
        if (hosts[i][0]) {
            fprintf(f, "%s\r\n", hosts[i]);
        }
    }
    fclose(f);
    if (!MoveFileExA(tmp_path, path, MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)) {
        DeleteFileA(tmp_path);
        Logf("auto-derived replace failed path=%s err=%lu\r\n", path, GetLastError());
        return false;
    }
    return true;
}

void PersistAutoDerivedFilter(const char* host, const char* reason) {
    if (!host || !host[0] || !g_dll_dir[0]) {
        return;
    }
    char hosts[128][64]{};
    int count = 0;
    if (!ReadAutoDerivedHosts(hosts, &count, static_cast<int>(sizeof(hosts) / sizeof(hosts[0])))) {
        Logf("auto-derived read failed host=%s\r\n", host);
        return;
    }
    for (int i = 0; i < count; ++i) {
        if (_stricmp(hosts[i], host) == 0) {
            Logf("steam auto-derived config unchanged host=%s reason=%s\r\n",
                 host, reason ? reason : "");
            return;
        }
    }
    if (count >= static_cast<int>(sizeof(hosts) / sizeof(hosts[0]))) {
        Logf("auto-derived config full, cannot add host=%s\r\n", host);
        return;
    }
    strncpy_s(hosts[count], 64, host, _TRUNCATE);
    count++;
    if (WriteAutoDerivedHostsAtomic(hosts, count)) {
        Logf("steam auto-derived config updated host=%s count=%d reason=%s\r\n",
             host, count, reason ? reason : "");
    }
}

DWORD WINAPI PersistAutoDerivedThread(LPVOID param) {
    AutoDerivedPersistJob* job = reinterpret_cast<AutoDerivedPersistJob*>(param);
    if (job) {
        PersistAutoDerivedFilter(job->host, job->reason);
        HeapFree(GetProcessHeap(), 0, job);
    }
    return 0;
}

void QueueAutoDerivedPersist(const char* host, const char* reason) {
    if (!host || !host[0]) {
        return;
    }
    AutoDerivedPersistJob* job = reinterpret_cast<AutoDerivedPersistJob*>(
        HeapAlloc(GetProcessHeap(), HEAP_ZERO_MEMORY, sizeof(AutoDerivedPersistJob)));
    if (!job) {
        return;
    }
    strncpy_s(job->host, host, _TRUNCATE);
    if (reason) {
        strncpy_s(job->reason, reason, _TRUNCATE);
    }
    HANDLE thread = CreateThread(nullptr, 0, PersistAutoDerivedThread, job, 0, nullptr);
    if (thread) {
        CloseHandle(thread);
    } else {
        HeapFree(GetProcessHeap(), 0, job);
    }
}

void ObserveDerivedAddressCandidate(DWORD root, const char* reason) {
    char host[64]{};
    if (!FormatSteamItemCanonicalHost(root, host, sizeof(host))) {
        return;
    }
    if (ShouldBlock(host)) {
        return;
    }

    DerivedAddressCandidate* slot = nullptr;
    for (int i = 0; i < static_cast<int>(sizeof(g_derived_candidates) / sizeof(g_derived_candidates[0])); ++i) {
        if (g_derived_candidates[i].host[0] && _stricmp(g_derived_candidates[i].host, host) == 0) {
            slot = &g_derived_candidates[i];
            break;
        }
        if (!g_derived_candidates[i].host[0] && !slot) {
            slot = &g_derived_candidates[i];
        }
    }
    if (!slot) {
        return;
    }
    if (!slot->host[0]) {
        strncpy_s(slot->host, host, _TRUNCATE);
    }
    slot->count++;

    const int promote_threshold = 3;
    if (!slot->promoted && slot->count >= promote_threshold) {
        slot->promoted = true;
        AddFilter(host);
        QueueAutoDerivedPersist(host, reason);
        Logf("steam auto-derived address promoted host=%s count=%d reason=%s total_filters=%d\r\n",
             host, slot->count, reason ? reason : "", g_filter_count);
    } else if (slot->count <= promote_threshold) {
        Logf("steam auto-derived candidate host=%s count=%d reason=%s\r\n",
             host, slot->count, reason ? reason : "");
    }
}

bool ShouldBlockSteamItemAddress(DWORD root, char* address, size_t address_size) {
    if (!FormatSteamItemAddress(root, address, address_size)) {
        return false;
    }
    return ShouldBlock(address);
}

bool HasHighPlayerLimitNearItem(DWORD root, int* max_players) {
    if (max_players) {
        *max_players = 0;
    }
    if (!IsReadablePointer(reinterpret_cast<const void*>(root + 0x70), 0x50)) {
        return false;
    }
    for (DWORD off = 0x70; off <= 0xA8; off += sizeof(DWORD)) {
        int value = *reinterpret_cast<const int*>(root + off);
        if (value > 8 && value <= 512) {
            if (max_players) {
                *max_players = value;
            }
            return true;
        }
    }
    return false;
}

bool HasExactHighPlayerLimitInGameServerItem(DWORD root, int* max_players) {
    if (max_players) {
        *max_players = 0;
    }
    if (!IsReadablePointer(reinterpret_cast<const void*>(root + 0x98), sizeof(int))) {
        return false;
    }
    int value = *reinterpret_cast<const int*>(root + 0x98);
    if (value > 8) {
        if (max_players) {
            *max_players = value;
        }
        return true;
    }
    return false;
}

int ReadIntNearItem(DWORD root, DWORD offset) {
    if (!IsReadablePointer(reinterpret_cast<const void*>(root + offset), sizeof(int))) {
        return -1;
    }
    return *reinterpret_cast<const int*>(root + offset);
}

void LogDefaultNameItemProbe(void* item, const char* name) {
    if (!item || !name || !EqualsAsciiStringNoCase(name, "Left 4 Dead 2")) {
        return;
    }
    if (g_steam_default_name_probe_logs >= 80) {
        return;
    }

    DWORD root = reinterpret_cast<DWORD>(item);
    char dir[64]{};
    char map_a[64]{};
    char map_b[64]{};
    char desc_a[96]{};
    char desc_b[96]{};
    char address[128]{};
    CopyAscii(dir, sizeof(dir), reinterpret_cast<const void*>(root + 0x0C), 60);
    CopyAscii(map_a, sizeof(map_a), reinterpret_cast<const void*>(root + 0x2C), 60);
    CopyAscii(map_b, sizeof(map_b), reinterpret_cast<const void*>(root + 0x2E), 60);
    CopyAscii(desc_a, sizeof(desc_a), reinterpret_cast<const void*>(root + 0x4C), 92);
    CopyAscii(desc_b, sizeof(desc_b), reinterpret_cast<const void*>(root + 0x4E), 92);
    FormatSteamItemAddress(root, address, sizeof(address));

    const char* map_hit = FindOfficialMapCodeInRange(root, 0x1000);
    int max_players = 0;
    bool high_limit = HasHighPlayerLimitNearItem(root, &max_players);

    g_steam_default_name_probe_logs++;
    Logf("steam default-name probe #%lu item=0x%p addr=%s dir=%s map2c=%s map2e=%s desc4c=%s desc4e=%s official=%s high_limit=%d max_guess=%d ints=90:%d 94:%d 98:%d 9c:%d a0:%d a4:%d a8:%d\r\n",
         g_steam_default_name_probe_logs, item, address, dir, map_a, map_b, desc_a, desc_b,
         map_hit ? map_hit : "", high_limit ? 1 : 0, max_players,
         ReadIntNearItem(root, 0x90), ReadIntNearItem(root, 0x94), ReadIntNearItem(root, 0x98),
         ReadIntNearItem(root, 0x9C), ReadIntNearItem(root, 0xA0), ReadIntNearItem(root, 0xA4),
         ReadIntNearItem(root, 0xA8));
}

const char* FindDisguisedEntryHeuristic(void* item, const char* name, char* reason, size_t reason_size) {
    if (reason && reason_size > 0) {
        reason[0] = 0;
    }
    if (!item || !name || !name[0]) {
        return nullptr;
    }

    bool default_name = EqualsAsciiStringNoCase(name, "Left 4 Dead 2");
    bool xy_hk_name = ContainsAsciiNoCase(name, "Valve Left4Dead 2 Hong Kong Server");
    if (!default_name && !xy_hk_name) {
        return nullptr;
    }

    DWORD root = reinterpret_cast<DWORD>(item);
    int max_players = 0;
    bool high_limit = HasExactHighPlayerLimitInGameServerItem(root, &max_players);
    if (!high_limit) {
        high_limit = HasHighPlayerLimitNearItem(root, &max_players);
    }
    if (!high_limit) {
        return nullptr;
    }

    char exact_map[64]{};
    const char* map = nullptr;
    if (CopyOfficialMapFromGameServerItem(root, exact_map, sizeof(exact_map))) {
        map = exact_map;
    } else {
        map = FindOfficialMapCodeInRange(root, 0x300);
    }
    if (!map) {
        if (default_name && max_players >= 128) {
            if (reason && reason_size > 0) {
                _snprintf_s(reason, reason_size, _TRUNCATE, "disguise:large_pool:max=%d", max_players);
            }
            return "disguised_default_large_pool";
        }
        return nullptr;
    }

    if (reason && reason_size > 0) {
        _snprintf_s(reason, reason_size, _TRUNCATE, "disguise:%s:max=%d", map, max_players);
    }
    return "disguised_default_official_map";
}

bool LooksLikeGameDetailsPayload(const unsigned char* data, size_t size) {
    if (!data || size < 17) {
        return false;
    }
    if (data[0] != 0xFF || data[1] != 0xFF || data[2] != 0xFF || data[3] != 0xFF) {
        return false;
    }
    return data[4] == 0x00 && ContainsBytes(data, size, "GameDetailsServer");
}

bool LooksLikeA2SInfoPayload(const unsigned char* data, size_t size) {
    if (!data || size < 8) {
        return false;
    }
    return data[0] == 0xFF && data[1] == 0xFF && data[2] == 0xFF && data[3] == 0xFF && data[4] == 0x49;
}

bool ShouldDropReceivedPayload(const char* buf, int len, char* reason, size_t reason_size) {
    if (reason && reason_size > 0) {
        reason[0] = 0;
    }
    if (!g_recvfrom_payload_drop || !buf || len <= 0) {
        return false;
    }

    const unsigned char* data = reinterpret_cast<const unsigned char*>(buf);
    size_t size = static_cast<size_t>(len);
    const bool game_details = LooksLikeGameDetailsPayload(data, size);
    const bool a2s_info = LooksLikeA2SInfoPayload(data, size);
    if (!game_details && !a2s_info) {
        return false;
    }

    const char* keyword = FindKeywordInBytes(data, size);
    if (!keyword) {
        return false;
    }
    if (reason && reason_size > 0) {
        _snprintf_s(reason, reason_size, _TRUNCATE, "%s keyword=%s len=%d",
                    game_details ? "GameDetailsServer" : "A2S_INFO", keyword, len);
    }
    return true;
}

bool WritePointerSafe(void** address, void* value) {
    if (!address || !IsReadablePointer(address, sizeof(void*))) {
        return false;
    }
    DWORD old_protect = 0;
    if (!VirtualProtect(address, sizeof(void*), PAGE_READWRITE, &old_protect)) {
        return false;
    }
    *address = value;
    DWORD ignored = 0;
    VirtualProtect(address, sizeof(void*), old_protect, &ignored);
    return true;
}

const char* FindKeywordInGameServerItem(void* item, char* name, size_t name_size) {
    if (name && name_size > 0) {
        name[0] = 0;
    }
    if (!item) {
        return nullptr;
    }
    DWORD root = reinterpret_cast<DWORD>(item);
    CopyAscii(name, name_size, reinterpret_cast<const void*>(root + 0xAC), 255);
    LogDefaultNameItemProbe(item, name);

    char heuristic[128]{};
    const char* hit = FindDisguisedEntryHeuristic(item, name, heuristic, sizeof(heuristic));
    if (hit) {
        g_steam_disguised_hits++;
        ObserveDerivedAddressCandidate(root, heuristic);
        if (name && name_size > 0 && heuristic[0]) {
            size_t used = strlen(name);
            if (used + 3 < name_size) {
                _snprintf_s(name + used, name_size - used, _TRUNCATE, " %s", heuristic);
            }
        }
        if (g_steam_disguised_hits <= 120) {
            Logf("steam disguised heuristic hit #%lu keyword=%s name=%s item=0x%p\r\n",
                 g_steam_disguised_hits, hit, name ? name : "", item);
        }
        return hit;
    }

    char address[128]{};
    if (ShouldBlockSteamItemAddress(root, address, sizeof(address))) {
        g_steam_address_hits++;
        if (name && name_size > 0 && address[0]) {
            size_t used = strlen(name);
            if (used + 3 < name_size) {
                _snprintf_s(name + used, name_size - used, _TRUNCATE, " addr:%s", address);
            }
        }
        if (g_steam_address_hits <= 120 || IsHighValueAddressForLog(address)) {
            Logf("steam item address hit #%lu address=%s name=%s item=0x%p\r\n",
                 g_steam_address_hits, address, name ? name : "", item);
        }
        return "steam_item_address";
    }

    hit = FindKeywordInRange(root + 0xAC, 256);
    if (hit) {
        return hit;
    }
    hit = FindKeywordInRange(root, 0x1000);
    if (hit) {
        return hit;
    }
    hit = FindKeywordNearObject(root);
    if (hit) {
        return hit;
    }
    return nullptr;
}

void* __fastcall HookedGetServerDetails(void* self, void*, void* request, int server) {
    GetServerDetailsFn original = g_get_server_details_original;
    if (!original) {
        return nullptr;
    }

    void* item = original(self, request, server);
    g_steam_details_seen++;

    char server_name[256]{};
    const char* keyword = FindKeywordInGameServerItem(item, server_name, sizeof(server_name));
    if (keyword) {
        g_steam_details_hidden++;
        if (g_steam_details_hidden <= 160) {
            Logf("steam GetServerDetails hidden #%lu seen=%lu request=0x%p server=%d keyword=%s name=%s item=0x%p\r\n",
                 g_steam_details_hidden, g_steam_details_seen, request, server, keyword, server_name, item);
        }
        return nullptr;
    }

    if (g_steam_details_seen <= 60) {
        Logf("steam GetServerDetails allow #%lu request=0x%p server=%d name=%s item=0x%p\r\n",
             g_steam_details_seen, request, server, server_name, item);
    }
    return item;
}

ServerRespondedFn FindOriginalServerResponded(void** vtable) {
    for (int i = 0; i < static_cast<int>(sizeof(g_response_vtables) / sizeof(g_response_vtables[0])); ++i) {
        if (g_response_vtables[i].vtable == vtable) {
            return g_response_vtables[i].original;
        }
    }
    return nullptr;
}

void __fastcall HookedServerResponded(void* self, void*, void* request, int server) {
    void** vtable = nullptr;
    if (self && IsReadablePointer(self, sizeof(void*))) {
        vtable = *reinterpret_cast<void***>(self);
    }
    ServerRespondedFn original = FindOriginalServerResponded(vtable);
    g_steam_responded_seen++;

    void* item = nullptr;
    if (g_get_server_details_original && g_steam_servers) {
        item = g_get_server_details_original(g_steam_servers, request, server);
    }

    char server_name[256]{};
    const char* keyword = FindKeywordInGameServerItem(item, server_name, sizeof(server_name));
    if (keyword) {
        g_steam_responded_dropped++;
        if (g_steam_responded_dropped <= 160) {
            Logf("steam ServerResponded dropped #%lu seen=%lu request=0x%p server=%d keyword=%s name=%s item=0x%p\r\n",
                 g_steam_responded_dropped, g_steam_responded_seen, request, server, keyword, server_name, item);
        }
        return;
    }

    if (g_steam_responded_seen <= 80) {
        Logf("steam ServerResponded allow #%lu request=0x%p server=%d name=%s item=0x%p\r\n",
             g_steam_responded_seen, request, server, server_name, item);
    }
    if (original) {
        original(self, request, server);
    }
}

bool PatchResponseVTable(void* response) {
    if (!g_steam_serverlist_drop || !response || !IsReadablePointer(response, sizeof(void*))) {
        return false;
    }
    void** vtable = *reinterpret_cast<void***>(response);
    if (!vtable || !IsReadablePointer(vtable, sizeof(void*) * 3)) {
        return false;
    }
    for (int i = 0; i < static_cast<int>(sizeof(g_response_vtables) / sizeof(g_response_vtables[0])); ++i) {
        if (g_response_vtables[i].vtable == vtable) {
            return true;
        }
    }

    ServerRespondedFn original = reinterpret_cast<ServerRespondedFn>(vtable[0]);
    if (!original || reinterpret_cast<void*>(original) == reinterpret_cast<void*>(&HookedServerResponded)) {
        return false;
    }
    if (!WritePointerSafe(&vtable[0], reinterpret_cast<void*>(&HookedServerResponded))) {
        return false;
    }
    for (int i = 0; i < static_cast<int>(sizeof(g_response_vtables) / sizeof(g_response_vtables[0])); ++i) {
        if (!g_response_vtables[i].vtable) {
            g_response_vtables[i].vtable = vtable;
            g_response_vtables[i].original = original;
            g_steam_response_patched++;
            Logf("steam response vtable patched #%lu response=0x%p vtable=0x%p original_ServerResponded=0x%p\r\n",
                 g_steam_response_patched, response, vtable, original);
            return true;
        }
    }
    return true;
}

void* HookRequestCommon(int slot, const char* name, void* self, unsigned int appid, void* filters, unsigned int filter_count, void* response) {
    g_steam_servers = self;
    g_steam_requests++;
    PatchResponseVTable(response);
    if (g_steam_requests <= 80) {
        Logf("steam request #%lu name=%s slot=%d appid=%u filters=0x%p count=%u response=0x%p self=0x%p\r\n",
             g_steam_requests, name, slot, appid, filters, filter_count, response, self);
    }
    RequestServerListFn original = (slot >= 0 && slot < 6) ? g_request_server_list_originals[slot] : nullptr;
    if (!original) {
        return nullptr;
    }
    return original(self, appid, filters, filter_count, response);
}

void* __fastcall HookRequestInternet(void* self, void*, unsigned int appid, void* filters, unsigned int filter_count, void* response) {
    return HookRequestCommon(0, "internet", self, appid, filters, filter_count, response);
}

void* __fastcall HookRequestFriends(void* self, void*, unsigned int appid, void* filters, unsigned int filter_count, void* response) {
    return HookRequestCommon(2, "friends", self, appid, filters, filter_count, response);
}

void* __fastcall HookRequestFavorites(void* self, void*, unsigned int appid, void* filters, unsigned int filter_count, void* response) {
    return HookRequestCommon(3, "favorites", self, appid, filters, filter_count, response);
}

void* __fastcall HookRequestHistory(void* self, void*, unsigned int appid, void* filters, unsigned int filter_count, void* response) {
    return HookRequestCommon(4, "history", self, appid, filters, filter_count, response);
}

void* __fastcall HookRequestSpectator(void* self, void*, unsigned int appid, void* filters, unsigned int filter_count, void* response) {
    return HookRequestCommon(5, "spectator", self, appid, filters, filter_count, response);
}

bool TryPatchSteamServerListInterface() {
    if (!g_steam_serverlist_drop) {
        return false;
    }
    HMODULE steam_api = GetModuleHandleA("steam_api.dll");
    if (!steam_api) {
        return false;
    }
    SteamAPIGetHSteamUserFn get_user = reinterpret_cast<SteamAPIGetHSteamUserFn>(GetProcAddress(steam_api, "SteamAPI_GetHSteamUser"));
    SteamInternalFindOrCreateUserInterfaceFn find_interface =
        reinterpret_cast<SteamInternalFindOrCreateUserInterfaceFn>(GetProcAddress(steam_api, "SteamInternal_FindOrCreateUserInterface"));
    if (!get_user || !find_interface) {
        return false;
    }
    int user = get_user();
    void* iface = find_interface(user, "SteamMatchMakingServers002");
    if (!iface || !IsReadablePointer(iface, sizeof(void*))) {
        return false;
    }
    void** vtable = *reinterpret_cast<void***>(iface);
    if (!vtable || !IsReadablePointer(vtable, sizeof(void*) * 8)) {
        return false;
    }

    g_steam_servers = iface;
    struct SlotPatch {
        int slot;
        void* hook;
        const char* name;
    };
    const SlotPatch slots[] = {
        {0, reinterpret_cast<void*>(&HookRequestInternet), "RequestInternetServerList"},
        {2, reinterpret_cast<void*>(&HookRequestFriends), "RequestFriendsServerList"},
        {3, reinterpret_cast<void*>(&HookRequestFavorites), "RequestFavoritesServerList"},
        {4, reinterpret_cast<void*>(&HookRequestHistory), "RequestHistoryServerList"},
        {5, reinterpret_cast<void*>(&HookRequestSpectator), "RequestSpectatorServerList"},
    };
    int patched = 0;
    for (int i = 0; i < static_cast<int>(sizeof(slots) / sizeof(slots[0])); ++i) {
        const SlotPatch& slot = slots[i];
        if (vtable[slot.slot] == slot.hook) {
            continue;
        }
        g_request_server_list_originals[slot.slot] = reinterpret_cast<RequestServerListFn>(vtable[slot.slot]);
        if (WritePointerSafe(&vtable[slot.slot], slot.hook)) {
            patched++;
            Logf("steam serverlist slot patched name=%s slot=%d original=0x%p hook=0x%p\r\n",
                 slot.name, slot.slot, g_request_server_list_originals[slot.slot], slot.hook);
        }
    }
    if (vtable[7] != reinterpret_cast<void*>(&HookedGetServerDetails)) {
        GetServerDetailsFn details = reinterpret_cast<GetServerDetailsFn>(vtable[7]);
        if (details && details != g_get_server_details_original) {
            g_get_server_details_original = details;
            Logf("steam serverlist slot observed name=GetServerDetails slot=7 original=0x%p hook=disabled\r\n",
                 g_get_server_details_original);
        }
    }
    static DWORD s_probe_logs = 0;
    if (patched > 0 || s_probe_logs < 3) {
        ++s_probe_logs;
        Logf("steam serverlist interface iface=0x%p vtable=0x%p patched=%d get_details=0x%p user=%d\r\n",
             iface, vtable, patched, g_get_server_details_original, user);
    }
    return patched > 0 || g_get_server_details_original != nullptr;
}

void NoteUniqueMatch(const char* connect) {
    if (!connect || !connect[0]) {
        return;
    }
    for (int i = 0; i < g_unique_match_count; ++i) {
        if (strcmp(connect, g_unique_matches[i]) == 0) {
            return;
        }
    }
    if (g_unique_match_count >= static_cast<int>(sizeof(g_unique_matches) / sizeof(g_unique_matches[0]))) {
        return;
    }
    strncpy_s(g_unique_matches[g_unique_match_count], connect, _TRUNCATE);
    g_unique_match_count++;
    Logf("new blocked connectstring #%d %s\r\n", g_unique_match_count, connect);
}

void NoteUniqueSeen(const char* connect) {
    if (!connect || !connect[0]) {
        return;
    }
    for (int i = 0; i < g_unique_seen_count; ++i) {
        if (strcmp(connect, g_unique_seen[i]) == 0) {
            return;
        }
    }
    if (g_unique_seen_count >= static_cast<int>(sizeof(g_unique_seen) / sizeof(g_unique_seen[0]))) {
        return;
    }
    strncpy_s(g_unique_seen[g_unique_seen_count], connect, _TRUNCATE);
    g_unique_seen_count++;
    Logf("seen connectstring #%d %s\r\n", g_unique_seen_count, connect);
}

bool WriteCode(void* address, const void* data, size_t size) {
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

bool Arm(Breakpoint* bp) {
    if (!bp->address || bp->armed) {
        return false;
    }
    BYTE trap = 0xCC;
    if (!WriteCode(bp->address, &trap, 1)) {
        return false;
    }
    bp->armed = true;
    return true;
}

bool Disarm(Breakpoint* bp) {
    if (!bp->address || !bp->armed) {
        return false;
    }
    if (!WriteCode(bp->address, &bp->original, 1)) {
        return false;
    }
    bp->armed = false;
    return true;
}

StepContext* GetStepContext(DWORD tid) {
    StepContext* empty = nullptr;
    for (int i = 0; i < static_cast<int>(sizeof(g_step_contexts) / sizeof(g_step_contexts[0])); ++i) {
        if (g_step_contexts[i].tid == tid) {
            return &g_step_contexts[i];
        }
        if (!empty && g_step_contexts[i].tid == 0) {
            empty = &g_step_contexts[i];
        }
    }
    return empty ? empty : &g_step_contexts[tid % (sizeof(g_step_contexts) / sizeof(g_step_contexts[0]))];
}

void SetStepBreakpoint(DWORD tid, Breakpoint* bp) {
    StepContext* step = GetStepContext(tid);
    if (!step) {
        return;
    }
    step->tid = tid;
    step->bp = bp;
}

Breakpoint* TakeStepBreakpoint(DWORD tid) {
    StepContext* step = GetStepContext(tid);
    if (!step || step->tid != tid || !step->bp) {
        return nullptr;
    }
    Breakpoint* bp = step->bp;
    step->bp = nullptr;
    return bp;
}

Breakpoint* FindBreakpoint(void* address) {
    if (g_refresh_entry_bp.address == address) {
        return &g_refresh_entry_bp;
    }
    if (g_table_update_bp.address == address) {
        return &g_table_update_bp;
    }
    if (g_details_entry_bp.address == address) {
        return &g_details_entry_bp;
    }
    if (g_table_slot2_bp.address == address) {
        return &g_table_slot2_bp;
    }
    if (g_connectstring_bp.address == address) {
        return &g_connectstring_bp;
    }
    if (g_update_emit_bp.address == address) {
        return &g_update_emit_bp;
    }
    if (g_primary_callback_bp.address == address) {
        return &g_primary_callback_bp;
    }
    if (g_dispatch_callback_bp.address == address) {
        return &g_dispatch_callback_bp;
    }
    if (g_client_groupserver_bp.address == address) {
        return &g_client_groupserver_bp;
    }
    if (g_client_row_field_bp.address == address) {
        return &g_client_row_field_bp;
    }
    if (g_client_row_submit_bp.address == address) {
        return &g_client_row_submit_bp;
    }
    if (g_client_server_name_bp.address == address) {
        return &g_client_server_name_bp;
    }
    if (g_client_server_row_bp.address == address) {
        return &g_client_server_row_bp;
    }
    if (g_client_thirdparty_panel_bp.address == address) {
        return &g_client_thirdparty_panel_bp;
    }
    if (g_sb_refresh_row_bp.address == address) {
        return &g_sb_refresh_row_bp;
    }
    if (g_sb_single_row_bp.address == address) {
        return &g_sb_single_row_bp;
    }
    return nullptr;
}

void LogTableProbe(const char* name, DWORD tid, const CONTEXT* ctx) {
    if (!name || !ctx || g_table_probe_logs >= 80) {
        return;
    }
    DWORD ret = 0;
    DWORD arg0 = 0;
    DWORD arg1 = 0;
#if defined(_M_IX86)
    ReadDword(ctx->Esp, &ret);
    ReadDword(ctx->Esp + 4, &arg0);
    ReadDword(ctx->Esp + 8, &arg1);
    const char* keyword = FindKeywordNearObject(ctx->Ecx);
    if (!keyword) {
        keyword = FindKeywordNearObject(arg0);
    }
    if (!keyword) {
        keyword = FindKeywordNearObject(arg1);
    }
    g_table_probe_logs++;
    Logf("table probe #%lu name=%s tid=%lu keyword=%s ecx=0x%08lX ret=0x%08lX arg0=0x%08lX arg1=0x%08lX\r\n",
         g_table_probe_logs, name, tid, keyword ? keyword : "", ctx->Ecx, ret, arg0, arg1);
#endif
}

bool LogPrimaryProbe(DWORD tid, const CONTEXT* ctx) {
    if (!ctx) {
        return false;
    }
#if defined(_M_IX86)
    KeywordLocation location{};
    if (!FindKeywordLocationNearObject(ctx->Esi, 512, 192, 4096, &location)) {
        if (g_primary_probe_logs < 32) {
            g_primary_probe_logs++;
            Logf("primary probe #%lu tid=%lu keyword= esi=0x%08lX ecx=0x%08lX edi=0x%08lX eax=0x%08lX\r\n",
                 g_primary_probe_logs, tid, ctx->Esi, ctx->Ecx, ctx->Edi, ctx->Eax);
        }
        return false;
    }
    if (g_primary_probe_logs < 160) {
        g_primary_probe_logs++;
        Logf("primary probe #%lu tid=%lu keyword=%s field=%d direct=%d ptr=0x%08lX esi=0x%08lX ecx=0x%08lX edi=0x%08lX eax=0x%08lX\r\n",
             g_primary_probe_logs, tid, location.keyword, location.field_index,
             location.direct ? 1 : 0, location.address, ctx->Esi, ctx->Ecx, ctx->Edi, ctx->Eax);
    }
    return true;
#else
    return false;
#endif
}

bool LogDispatchProbe(DWORD tid, const CONTEXT* ctx) {
    if (!ctx) {
        return false;
    }
    DWORD ret = 0;
    DWORD arg0 = 0;
    DWORD arg1 = 0;
#if defined(_M_IX86)
    ReadDword(ctx->Esp, &ret);
    ReadDword(ctx->Esp + 4, &arg0);
    ReadDword(ctx->Esp + 8, &arg1);
    KeywordLocation location{};
    const char* keyword = nullptr;
    const char* source = "ecx";
    if (FindKeywordLocationNearObject(ctx->Ecx, 512, 96, 2048, &location)) {
        keyword = location.keyword;
    } else if (FindKeywordLocationNearObject(ctx->Esi, 512, 192, 4096, &location)) {
        keyword = location.keyword;
        source = "esi";
    } else if (FindKeywordLocationNearObject(ctx->Edi, 512, 192, 4096, &location)) {
        keyword = location.keyword;
        source = "edi";
    } else if (FindKeywordLocationNearObject(ctx->Ebx, 512, 192, 4096, &location)) {
        keyword = location.keyword;
        source = "ebx";
    } else if (FindKeywordLocationNearObject(arg0, 512, 192, 4096, &location)) {
        keyword = location.keyword;
        source = "arg0";
    } else if (FindKeywordLocationNearObject(arg1, 512, 192, 4096, &location)) {
        keyword = location.keyword;
        source = "arg1";
    }
    bool should_log = false;
    if (keyword) {
        should_log = g_dispatch_probe_logs < 240;
    } else {
        should_log = g_dispatch_probe_logs < 64;
    }
    if (should_log) {
        g_dispatch_probe_logs++;
        Logf("dispatch probe #%lu tid=%lu keyword=%s source=%s field=%d direct=%d ptr=0x%08lX ecx=0x%08lX esi=0x%08lX edi=0x%08lX ebx=0x%08lX ret=0x%08lX arg0=0x%08lX arg1=0x%08lX\r\n",
             g_dispatch_probe_logs, tid, keyword ? keyword : "", keyword ? source : "",
             keyword ? location.field_index : -1, keyword && location.direct ? 1 : 0,
             keyword ? location.address : 0, ctx->Ecx, ctx->Esi, ctx->Edi, ctx->Ebx, ret, arg0, arg1);
    }
#endif
    return keyword != nullptr;
}

bool IsClientProbe(Breakpoint* bp) {
    return bp == &g_client_groupserver_bp ||
           bp == &g_client_row_field_bp ||
           bp == &g_client_row_submit_bp ||
           bp == &g_client_server_name_bp ||
           bp == &g_client_server_row_bp ||
           bp == &g_client_thirdparty_panel_bp;
}

bool IsServerBrowserProbe(Breakpoint* bp) {
    return bp == &g_sb_refresh_row_bp ||
           bp == &g_sb_single_row_bp;
}

bool IsRowCreationProbeName(const char* name) {
    return name &&
           (strcmp(name, "client_row_field_candidate") == 0 ||
            strcmp(name, "client_row_submit_candidate") == 0);
}

bool LogClientProbe(const char* name, DWORD tid, const CONTEXT* ctx) {
    if (!name || !ctx) {
        return false;
    }
    DWORD ret = 0;
    DWORD arg0 = 0;
    DWORD arg1 = 0;
#if defined(_M_IX86)
    ReadDword(ctx->Esp, &ret);
    ReadDword(ctx->Esp + 4, &arg0);
    ReadDword(ctx->Esp + 8, &arg1);
    KeywordLocation location{};
    const char* keyword = nullptr;
    const char* source = "ecx";
    const bool row_creation_probe = IsRowCreationProbeName(name);
    const size_t root_bytes = row_creation_probe ? 1024 : 512;
    const int ecx_fields = row_creation_probe ? 192 : 64;
    const int arg0_fields = row_creation_probe ? 256 : 64;
    const int arg1_fields = row_creation_probe ? 256 : 128;
    const size_t field_bytes = row_creation_probe ? 4096 : 1024;
    const size_t arg1_bytes = row_creation_probe ? 4096 : 2048;
    if (FindKeywordLocationNearObject(ctx->Ecx, root_bytes, ecx_fields, field_bytes, &location)) {
        keyword = location.keyword;
    } else if (FindKeywordLocationNearObject(arg0, root_bytes, arg0_fields, field_bytes, &location)) {
        keyword = location.keyword;
        source = "arg0";
    } else if (FindKeywordLocationNearObject(arg1, root_bytes, arg1_fields, arg1_bytes, &location)) {
        keyword = location.keyword;
        source = "arg1";
    }
    if (!keyword) {
        if (row_creation_probe && g_client_call_probe_logs < 96) {
            g_client_call_probe_logs++;
            Logf("client ui call probe #%lu name=%s tid=%lu keyword=0 ecx=0x%08lX ret=0x%08lX arg0=0x%08lX arg1=0x%08lX\r\n",
                 g_client_call_probe_logs, name, tid, ctx->Ecx, ret, arg0, arg1);
        }
        return false;
    }
    if (g_client_probe_logs >= 240) {
        return true;
    }
    int ui_neutralized = 0;
    if (g_neutralize_keywords && strcmp(name, "client_server_name_candidate") == 0) {
        ui_neutralized = NeutralizeKeywordsInRange(location.address, 2048);
        g_client_neutralized += static_cast<DWORD>(ui_neutralized);
    }
    g_client_probe_logs++;
    Logf("client ui keyword probe #%lu name=%s tid=%lu keyword=%s source=%s field=%d direct=%d ptr=0x%08lX ui_neutralized=%d total_ui_neutralized=%lu ecx=0x%08lX ret=0x%08lX arg0=0x%08lX arg1=0x%08lX\r\n",
         g_client_probe_logs, name, tid, keyword, source, location.field_index, location.direct ? 1 : 0,
         location.address, ui_neutralized, g_client_neutralized, ctx->Ecx, ret, arg0, arg1);
#endif
    return true;
}

EntryContext* GetEntryContext(DWORD tid) {
    EntryContext* empty = nullptr;
    for (int i = 0; i < static_cast<int>(sizeof(g_entry_contexts) / sizeof(g_entry_contexts[0])); ++i) {
        if (g_entry_contexts[i].tid == tid) {
            return &g_entry_contexts[i];
        }
        if (!empty && g_entry_contexts[i].tid == 0) {
            empty = &g_entry_contexts[i];
        }
    }
    return empty ? empty : &g_entry_contexts[tid % (sizeof(g_entry_contexts) / sizeof(g_entry_contexts[0]))];
}

void SaveEntryContext(DWORD tid, const CONTEXT* ctx) {
    if (!ctx) {
        return;
    }
    EntryContext* entry = GetEntryContext(tid);
    if (!entry) {
        return;
    }
    DWORD ret = 0;
    DWORD arg0 = 0;
#if defined(_M_IX86)
    ReadDword(ctx->Esp, &ret);
    ReadDword(ctx->Esp + 4, &arg0);
    entry->ecx = ctx->Ecx;
#endif
    entry->tid = tid;
    entry->seq = ++g_entry_hits;
    entry->ret = ret;
    entry->arg0 = arg0;
    entry->keyword_block = false;
    entry->keyword[0] = 0;
    KeywordLocation location{};
    const char* source = "";
    if (FindKeywordLocationNearObject(entry->ecx, 512, 64, 1024, &location)) {
        source = "ecx";
    } else if (FindKeywordLocationNearObject(entry->arg0, 512, 128, 2048, &location)) {
        source = "arg0";
    }
    if (location.keyword) {
        entry->keyword_block = true;
        strncpy_s(entry->keyword, location.keyword, _TRUNCATE);
        int neutralized_ecx = 0;
        int neutralized_arg0 = 0;
        int neutralized_arg0_deep = 0;
        char deep_summary[160]{};
        if (g_neutralize_keywords) {
            neutralized_ecx = NeutralizeKeywordsNearObject(entry->ecx);
            neutralized_arg0 = NeutralizeKeywordsNearObject(entry->arg0);
            neutralized_arg0_deep = NeutralizeHotArg0SecondLevel(entry->arg0, deep_summary, sizeof(deep_summary));
            int neutralized = neutralized_ecx + neutralized_arg0 + neutralized_arg0_deep;
            g_neutralized += static_cast<DWORD>(neutralized);
        }
        Logf("entry keyword hit seq=%lu keyword=%s source=%s field=%d direct=%d ptr=0x%08lX neutralized_ecx=%d neutralized_arg0=%d neutralized_arg0_deep=%d deep=%s total_neutralized=%lu ecx=0x%08lX arg0=0x%08lX ret=0x%08lX\r\n",
             entry->seq, entry->keyword, source, location.field_index, location.direct ? 1 : 0, location.address,
             neutralized_ecx, neutralized_arg0, neutralized_arg0_deep, deep_summary, g_neutralized, entry->ecx, entry->arg0, entry->ret);
    }
}

LONG CALLBACK VehHandler(EXCEPTION_POINTERS* info) {
    if (!info || !info->ExceptionRecord || !info->ContextRecord) {
        return EXCEPTION_CONTINUE_SEARCH;
    }

    DWORD code = info->ExceptionRecord->ExceptionCode;
    CONTEXT* ctx = info->ContextRecord;
    DWORD tid = GetCurrentThreadId();

    if (code == EXCEPTION_BREAKPOINT) {
        void* hit = info->ExceptionRecord->ExceptionAddress;
        Breakpoint* bp = FindBreakpoint(hit);
        if (!bp) {
            return EXCEPTION_CONTINUE_SEARCH;
        }

        if (bp == &g_refresh_entry_bp) {
            g_refreshes++;
            Logf("group refresh #%lu begin seen=%lu skipped=%lu unique_blocked=%d\r\n",
                 g_refreshes, g_seen, g_skipped, g_unique_match_count);
        } else if (bp == &g_table_update_bp) {
            LogTableProbe(bp->name, tid, ctx);
        } else if (bp == &g_details_entry_bp) {
            SaveEntryContext(tid, ctx);
        } else if (bp == &g_table_slot2_bp) {
            LogTableProbe(bp->name, tid, ctx);
    } else if (bp == &g_primary_callback_bp) {
            LogPrimaryProbe(tid, ctx);
        } else if (bp == &g_dispatch_callback_bp) {
            bool dispatch_keyword = LogDispatchProbe(tid, ctx);
            if (dispatch_keyword && g_dispatch_skip) {
                g_skipped++;
#if defined(_M_IX86)
                ctx->Eip = reinterpret_cast<DWORD>(g_matchmaking_base + 0x16F85);
#endif
                if (g_skipped <= 320) {
                    Logf("dispatch skipped callback #%lu target=0x16F85\r\n", g_skipped);
                }
                return EXCEPTION_CONTINUE_EXECUTION;
            }
        } else if (IsClientProbe(bp)) {
            bool client_keyword = LogClientProbe(bp->name, tid, ctx);
            if (client_keyword && g_client_skip_name && bp == &g_client_server_name_bp) {
                DWORD ret = 0;
                ReadDword(ctx->Esp, &ret);
                if (ret) {
                    int marked = 0;
                    if (g_client_mark_hidden) {
                        marked += WriteByteSafe(ctx->Ecx + 0x3C8, 1) ? 1 : 0;
                        marked += WriteByteSafe(ctx->Ecx + 0x3C9, 1) ? 1 : 0;
                        marked += WriteByteSafe(ctx->Ecx + 0x3CA, 1) ? 1 : 0;
                    }
                    g_skipped++;
#if defined(_M_IX86)
                    ctx->Eip = ret;
                    ctx->Esp += 8;
                    ctx->Eax = 0;
#endif
                    Logf("client skipped server-name function #%lu ret=0x%08lX marked_hidden=%d ecx=0x%08lX\r\n",
                         g_skipped, ret, marked, ctx->Ecx);
                    return EXCEPTION_CONTINUE_EXECUTION;
                }
            }
        } else if (IsServerBrowserProbe(bp)) {
            char summary[96]{};
            const char* keyword = FindKeywordInServerBrowserEntry(ctx->Esi, summary, sizeof(summary));
            if (keyword) {
                if (g_serverbrowser_probe_logs < 80) {
                    g_serverbrowser_probe_logs++;
                    Logf("serverbrowser keyword probe #%lu name=%s keyword=%s field=%s esi=0x%08lX ecx=0x%08lX ebx=0x%08lX edi=0x%08lX\r\n",
                         g_serverbrowser_probe_logs, bp->name, keyword, summary, ctx->Esi, ctx->Ecx, ctx->Ebx, ctx->Edi);
                }
                if (g_serverbrowser_row_skip && g_serverbrowser_base) {
                    g_skipped++;
#if defined(_M_IX86)
                    if (bp == &g_sb_refresh_row_bp) {
                        ctx->Eip = reinterpret_cast<DWORD>(g_serverbrowser_base + 0x76F8);
                    } else {
                        ctx->Eip = reinterpret_cast<DWORD>(g_serverbrowser_base + 0xC9A5);
                    }
#endif
                    if (g_skipped <= 360) {
                        Logf("serverbrowser skipped row #%lu name=%s keyword=%s target=0x%08lX\r\n",
                             g_skipped, bp->name, keyword, ctx->Eip);
                    }
                    return EXCEPTION_CONTINUE_EXECUTION;
                }
            }
        } else if (bp == &g_connectstring_bp) {
            char connect[160];
            CopyAscii(connect, sizeof(connect), reinterpret_cast<const void*>(ctx->Eax), 150);
            g_seen++;
            NoteUniqueSeen(connect);
            EntryContext* entry = GetEntryContext(tid);
            bool keyword_block = entry && entry->tid == tid && entry->keyword_block;
            if (ShouldBlock(connect) || keyword_block) {
                NoteUniqueMatch(connect);
                if (entry && entry->tid == tid) {
                    Logf("match #%lu tid=%lu connect=%s reason=%s keyword=%s entry_seq=%lu entry_ecx=0x%08lX entry_ret=0x%08lX entry_arg0=0x%08lX\r\n",
                         g_seen, tid, connect, keyword_block ? "keyword" : "connectstring",
                         entry->keyword, entry->seq, entry->ecx, entry->ret, entry->arg0);
                } else {
                    Logf("match #%lu tid=%lu connect=%s reason=connectstring entry_seq=none\r\n", g_seen, tid, connect);
                }
                if (g_early_skip) {
                    g_skipped++;
#if defined(_M_IX86)
                    ctx->Eip = reinterpret_cast<DWORD>(g_matchmaking_base + 0x2203B);
#endif
                    Logf("early skipped detail consume #%lu target=0x2203B\r\n", g_skipped);
                    return EXCEPTION_CONTINUE_EXECUTION;
                }
                g_skip_thread = tid;
            }
        } else if (bp == &g_update_emit_bp) {
            if (g_skip_thread == tid) {
                g_skip_thread = 0;
                g_skipped++;
                Disarm(bp);
#if defined(_M_IX86)
                ctx->Eip = reinterpret_cast<DWORD>(g_matchmaking_base + 0x22039);
#endif
                Arm(bp);
                Logf("skipped server/update #%lu\r\n", g_skipped);
                return EXCEPTION_CONTINUE_EXECUTION;
            }
        }

        Disarm(bp);
#if defined(_M_IX86)
        ctx->Eip = reinterpret_cast<DWORD>(bp->address);
        ctx->EFlags |= 0x100;
#endif
        SetStepBreakpoint(tid, bp);
        return EXCEPTION_CONTINUE_EXECUTION;
    }

    if (code == EXCEPTION_SINGLE_STEP) {
        Breakpoint* bp = TakeStepBreakpoint(tid);
        if (!bp) {
            return EXCEPTION_CONTINUE_SEARCH;
        }
        Arm(bp);
        return EXCEPTION_CONTINUE_EXECUTION;
    }

    return EXCEPTION_CONTINUE_SEARCH;
}

bool IsKnownBinServerBrowserPath(const char* path) {
    if (!path || !path[0]) {
        return false;
    }
    char lower[MAX_PATH]{};
    strncpy_s(lower, path, _TRUNCATE);
    _strlwr_s(lower, sizeof(lower));
    return strstr(lower, "\\left 4 dead 2\\bin\\serverbrowser.dll") != nullptr;
}

HMODULE FindBinServerBrowserModule(char* module_path, size_t module_path_size) {
    if (module_path && module_path_size > 0) {
        module_path[0] = 0;
    }
    HMODULE modules[512]{};
    DWORD bytes_needed = 0;
    if (!EnumProcessModules(GetCurrentProcess(), modules, sizeof(modules), &bytes_needed)) {
        return nullptr;
    }
    DWORD count = bytes_needed / sizeof(HMODULE);
    if (count > static_cast<DWORD>(sizeof(modules) / sizeof(modules[0]))) {
        count = static_cast<DWORD>(sizeof(modules) / sizeof(modules[0]));
    }
    for (DWORD i = 0; i < count; ++i) {
        char path[MAX_PATH]{};
        if (!GetModuleFileNameA(modules[i], path, MAX_PATH)) {
            continue;
        }
        if (!IsKnownBinServerBrowserPath(path)) {
            continue;
        }
        if (module_path && module_path_size > 0) {
            _snprintf_s(module_path, module_path_size, _TRUNCATE, "%s", path);
        }
        return modules[i];
    }
    return nullptr;
}

bool TryArmServerBrowser() {
    if (g_serverbrowser_base) {
        return true;
    }
    char module_path[MAX_PATH]{};
    HMODULE module = FindBinServerBrowserModule(module_path, sizeof(module_path));
    if (!module) {
        return false;
    }

    g_serverbrowser_base = reinterpret_cast<BYTE*>(module);
    g_sb_refresh_row_bp.address = g_serverbrowser_base + g_sb_refresh_row_bp.rva;
    g_sb_single_row_bp.address = g_serverbrowser_base + g_sb_single_row_bp.rva;
    g_sb_refresh_row_bp.original = *g_sb_refresh_row_bp.address;
    g_sb_single_row_bp.original = *g_sb_single_row_bp.address;
    bool refresh_armed = Arm(&g_sb_refresh_row_bp);
    bool single_armed = Arm(&g_sb_single_row_bp);
    Logf("serverbrowser module armed base=0x%p path=%s refresh_row=0x%p armed=%d single_row=0x%p armed=%d\r\n",
         g_serverbrowser_base, module_path, g_sb_refresh_row_bp.address, refresh_armed ? 1 : 0,
         g_sb_single_row_bp.address, single_armed ? 1 : 0);
    return refresh_armed || single_armed;
}

int __stdcall HookedRecvFrom(UINT_PTR s, char* buf, int len, int flags, void* from, int* fromlen) {
    RecvFromFn original = g_recvfrom_original;
    if (!original) {
        return -1;
    }
    int result = original(s, buf, len, flags, from, fromlen);
    if (result <= 0) {
        return result;
    }

    g_recvfrom_seen++;
    char reason[128]{};
    if (!ShouldDropReceivedPayload(buf, result, reason, sizeof(reason))) {
        return result;
    }

    g_recvfrom_dropped++;
    if (g_recvfrom_dropped <= 120) {
        Logf("recvfrom dropped payload #%lu seen=%lu %s\r\n", g_recvfrom_dropped, g_recvfrom_seen, reason);
    }
    if (g_wsa_set_last_error) {
        g_wsa_set_last_error(10035);
    } else {
        SetLastError(10035);
    }
    return -1;
}

bool IsReadableImageModule(HMODULE module) {
    if (!module) {
        return false;
    }
    const BYTE* base = reinterpret_cast<const BYTE*>(module);
    if (!IsReadablePointer(base, sizeof(IMAGE_DOS_HEADER))) {
        return false;
    }
    const IMAGE_DOS_HEADER* dos = reinterpret_cast<const IMAGE_DOS_HEADER*>(base);
    if (dos->e_magic != IMAGE_DOS_SIGNATURE) {
        return false;
    }
    if (!IsReadablePointer(base + dos->e_lfanew, sizeof(IMAGE_NT_HEADERS))) {
        return false;
    }
    const IMAGE_NT_HEADERS* nt = reinterpret_cast<const IMAGE_NT_HEADERS*>(base + dos->e_lfanew);
    return nt->Signature == IMAGE_NT_SIGNATURE;
}

int PatchRecvFromImportInModule(HMODULE module) {
    if (!IsReadableImageModule(module)) {
        return 0;
    }
    BYTE* base = reinterpret_cast<BYTE*>(module);
    IMAGE_DOS_HEADER* dos = reinterpret_cast<IMAGE_DOS_HEADER*>(base);
    IMAGE_NT_HEADERS* nt = reinterpret_cast<IMAGE_NT_HEADERS*>(base + dos->e_lfanew);
    IMAGE_DATA_DIRECTORY imports_dir = nt->OptionalHeader.DataDirectory[IMAGE_DIRECTORY_ENTRY_IMPORT];
    if (!imports_dir.VirtualAddress || !imports_dir.Size) {
        return 0;
    }

    IMAGE_IMPORT_DESCRIPTOR* imports = reinterpret_cast<IMAGE_IMPORT_DESCRIPTOR*>(base + imports_dir.VirtualAddress);
    int patched = 0;
    for (; imports->Name; ++imports) {
        const char* dll_name = reinterpret_cast<const char*>(base + imports->Name);
        if (!dll_name || _stricmp(dll_name, "ws2_32.dll") != 0) {
            continue;
        }
        IMAGE_THUNK_DATA* names = imports->OriginalFirstThunk
            ? reinterpret_cast<IMAGE_THUNK_DATA*>(base + imports->OriginalFirstThunk)
            : nullptr;
        IMAGE_THUNK_DATA* funcs = reinterpret_cast<IMAGE_THUNK_DATA*>(base + imports->FirstThunk);
        if (!names || !funcs) {
            continue;
        }
        for (; names->u1.AddressOfData && funcs->u1.Function; ++names, ++funcs) {
            if (names->u1.Ordinal & IMAGE_ORDINAL_FLAG32) {
                continue;
            }
            IMAGE_IMPORT_BY_NAME* import_name = reinterpret_cast<IMAGE_IMPORT_BY_NAME*>(base + names->u1.AddressOfData);
            if (!import_name || strcmp(reinterpret_cast<const char*>(import_name->Name), "recvfrom") != 0) {
                continue;
            }
            if (reinterpret_cast<void*>(funcs->u1.Function) == reinterpret_cast<void*>(&HookedRecvFrom)) {
                continue;
            }
            DWORD old_protect = 0;
            if (!VirtualProtect(&funcs->u1.Function, sizeof(funcs->u1.Function), PAGE_READWRITE, &old_protect)) {
                continue;
            }
            funcs->u1.Function = reinterpret_cast<ULONG_PTR>(&HookedRecvFrom);
            DWORD ignored = 0;
            VirtualProtect(&funcs->u1.Function, sizeof(funcs->u1.Function), old_protect, &ignored);
            patched++;
        }
    }
    return patched;
}

int PatchRecvFromImports() {
    HMODULE ws2 = GetModuleHandleA("ws2_32.dll");
    if (!ws2) {
        return 0;
    }
    if (!g_recvfrom_original) {
        g_recvfrom_original = reinterpret_cast<RecvFromFn>(GetProcAddress(ws2, "recvfrom"));
    }
    if (!g_wsa_set_last_error) {
        g_wsa_set_last_error = reinterpret_cast<WSASetLastErrorFn>(GetProcAddress(ws2, "WSASetLastError"));
    }
    if (!g_recvfrom_original) {
        return 0;
    }

    HMODULE modules[512]{};
    DWORD bytes_needed = 0;
    if (!EnumProcessModules(GetCurrentProcess(), modules, sizeof(modules), &bytes_needed)) {
        return 0;
    }
    DWORD count = bytes_needed / sizeof(HMODULE);
    if (count > static_cast<DWORD>(sizeof(modules) / sizeof(modules[0]))) {
        count = static_cast<DWORD>(sizeof(modules) / sizeof(modules[0]));
    }
    int patched = 0;
    for (DWORD i = 0; i < count; ++i) {
        patched += PatchRecvFromImportInModule(modules[i]);
    }
    if (patched > 0) {
        g_recvfrom_patched += static_cast<DWORD>(patched);
        Logf("recvfrom IAT patched entries=%d total=%lu original=0x%p\r\n", patched, g_recvfrom_patched, g_recvfrom_original);
    }
    return patched;
}

bool UsesBreakpointProbes() {
    return g_early_skip ||
           g_neutralize_keywords ||
           g_client_skip_name ||
           g_dispatch_skip ||
           g_client_mark_hidden ||
           g_serverbrowser_row_skip;
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
    strncpy_s(g_dll_dir, dll_path, _TRUNCATE);

    char log_path[MAX_PATH]{};
    _snprintf_s(log_path, sizeof(log_path), _TRUNCATE, "%smatchmaking_row_filter.log", dll_path);
    g_log = CreateFileA(log_path, GENERIC_WRITE, FILE_SHARE_READ, nullptr, CREATE_ALWAYS, FILE_ATTRIBUTE_NORMAL, nullptr);
    Logf("matchmaking_row_filter loaded pid=%lu\r\n", GetCurrentProcessId());

    LoadFilters(dll_path);
    LoadKeywords(dll_path);
    LoadMode(dll_path);
    if (g_recvfrom_payload_drop) {
        PatchRecvFromImports();
    }
    if (g_steam_serverlist_drop) {
        TryPatchSteamServerListInterface();
    }

    if (!UsesBreakpointProbes()) {
        LogRaw("breakpoint probes disabled for current mode; using lightweight import/vtable hooks only.\r\n");
        for (int i = 0; i < 180; ++i) {
            Sleep(1000);
            if (g_recvfrom_payload_drop) {
                PatchRecvFromImports();
            }
            if (g_steam_serverlist_drop) {
                TryPatchSteamServerListInterface();
            }
        }
        return 0;
    }

    HMODULE matchmaking = nullptr;
    for (int i = 0; i < 200 && !matchmaking; ++i) {
        matchmaking = GetModuleHandleA("matchmaking.dll");
        if (!matchmaking) {
            Sleep(100);
        }
    }
    if (!matchmaking) {
        LogRaw("matchmaking.dll not loaded; filter disabled.\r\n");
        return 0;
    }
    g_matchmaking_base = reinterpret_cast<BYTE*>(matchmaking);

    HMODULE client = nullptr;
    for (int i = 0; i < 50 && !client; ++i) {
        client = GetModuleHandleA("client.dll");
        if (!client) {
            Sleep(100);
        }
    }
    if (client) {
        g_client_base = reinterpret_cast<BYTE*>(client);
        g_client_groupserver_bp.address = g_client_base + g_client_groupserver_bp.rva;
        g_client_row_field_bp.address = g_client_base + g_client_row_field_bp.rva;
        g_client_row_submit_bp.address = g_client_base + g_client_row_submit_bp.rva;
        g_client_server_name_bp.address = g_client_base + g_client_server_name_bp.rva;
        g_client_server_row_bp.address = g_client_base + g_client_server_row_bp.rva;
        g_client_thirdparty_panel_bp.address = g_client_base + g_client_thirdparty_panel_bp.rva;
        g_client_groupserver_bp.original = *g_client_groupserver_bp.address;
        g_client_row_field_bp.original = *g_client_row_field_bp.address;
        g_client_row_submit_bp.original = *g_client_row_submit_bp.address;
        g_client_server_name_bp.original = *g_client_server_name_bp.address;
        g_client_server_row_bp.original = *g_client_server_row_bp.address;
        g_client_thirdparty_panel_bp.original = *g_client_thirdparty_panel_bp.address;
    } else {
        LogRaw("client.dll not loaded; client UI probes disabled.\r\n");
    }

    g_refresh_entry_bp.address = g_matchmaking_base + g_refresh_entry_bp.rva;
    g_table_update_bp.address = g_matchmaking_base + g_table_update_bp.rva;
    g_details_entry_bp.address = g_matchmaking_base + g_details_entry_bp.rva;
    g_table_slot2_bp.address = g_matchmaking_base + g_table_slot2_bp.rva;
    g_connectstring_bp.address = g_matchmaking_base + g_connectstring_bp.rva;
    g_update_emit_bp.address = g_matchmaking_base + g_update_emit_bp.rva;
    g_primary_callback_bp.address = g_matchmaking_base + g_primary_callback_bp.rva;
    g_dispatch_callback_bp.address = g_matchmaking_base + g_dispatch_callback_bp.rva;
    g_refresh_entry_bp.original = *g_refresh_entry_bp.address;
    g_table_update_bp.original = *g_table_update_bp.address;
    g_details_entry_bp.original = *g_details_entry_bp.address;
    g_table_slot2_bp.original = *g_table_slot2_bp.address;
    g_connectstring_bp.original = *g_connectstring_bp.address;
    g_update_emit_bp.original = *g_update_emit_bp.address;
    g_primary_callback_bp.original = *g_primary_callback_bp.address;
    g_dispatch_callback_bp.original = *g_dispatch_callback_bp.address;

    g_veh = AddVectoredExceptionHandler(1, VehHandler);
    if (!g_veh) {
        LogRaw("AddVectoredExceptionHandler failed.\r\n");
        return 0;
    }
    Arm(&g_refresh_entry_bp);
    Arm(&g_table_update_bp);
    Arm(&g_details_entry_bp);
    Arm(&g_table_slot2_bp);
    Arm(&g_connectstring_bp);
    Arm(&g_update_emit_bp);
    Arm(&g_primary_callback_bp);
    Arm(&g_dispatch_callback_bp);
    if (g_client_base) {
        Arm(&g_client_groupserver_bp);
        Arm(&g_client_row_field_bp);
        Arm(&g_client_row_submit_bp);
        Arm(&g_client_server_name_bp);
        Arm(&g_client_server_row_bp);
        Arm(&g_client_thirdparty_panel_bp);
    }
    TryArmServerBrowser();
    Logf("armed refresh=0x%p table_update=0x%p details_entry=0x%p table_slot2=0x%p connectstring=0x%p update_emit=0x%p primary=0x%p dispatch=0x%p client_group=0x%p client_row_field=0x%p client_row_submit=0x%p client_name=0x%p client_row=0x%p client_panel=0x%p serverbrowser_refresh=0x%p serverbrowser_single=0x%p filters=%d keywords=%d\r\n",
         g_refresh_entry_bp.address, g_table_update_bp.address, g_details_entry_bp.address,
         g_table_slot2_bp.address, g_connectstring_bp.address, g_update_emit_bp.address,
         g_primary_callback_bp.address, g_dispatch_callback_bp.address,
         g_client_groupserver_bp.address, g_client_row_field_bp.address, g_client_row_submit_bp.address,
         g_client_server_name_bp.address, g_client_server_row_bp.address,
         g_client_thirdparty_panel_bp.address,
         g_sb_refresh_row_bp.address, g_sb_single_row_bp.address,
         g_filter_count, g_keyword_count);
    for (int i = 0; i < 600 && !g_serverbrowser_base; ++i) {
        Sleep(100);
        if (g_recvfrom_payload_drop && (i % 10) == 0) {
            PatchRecvFromImports();
        }
        if (g_steam_serverlist_drop && (i % 10) == 0) {
            TryPatchSteamServerListInterface();
        }
        if (TryArmServerBrowser()) {
            break;
        }
    }
    if (g_recvfrom_payload_drop) {
        for (int i = 0; i < 60; ++i) {
            Sleep(1000);
            PatchRecvFromImports();
        }
    }
    if (g_steam_serverlist_drop) {
        for (int i = 0; i < 60; ++i) {
            Sleep(1000);
            TryPatchSteamServerListInterface();
        }
    }
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
