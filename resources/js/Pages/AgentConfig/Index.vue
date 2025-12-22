<template>
    <AppLayout>
        <div class="bg-white dark:bg-gray-800 shadow-lg dark:shadow-gray-900/50 overflow-hidden sm:rounded-lg border border-gray-200 dark:border-gray-700">
            <div class="px-4 py-5 sm:px-6 border-b border-gray-200 dark:border-gray-700">
                <div class="flex justify-between items-center">
                    <div>
                        <h3 class="text-lg leading-6 font-medium text-gray-900 dark:text-white">
                            Agent Configuration
                        </h3>
                        <p class="mt-1 max-w-2xl text-sm text-gray-500 dark:text-gray-400">
                            Manage settings and view system logs.
                        </p>
                    </div>

                    <!-- Tab Navigation in Header -->
                    <div class="flex gap-2">
                        <button
                            @click="activeTab = 'settings'"
                            :class="[
                                activeTab === 'settings'
                                    ? 'bg-indigo-600 text-white'
                                    : 'bg-gray-700 text-gray-200 hover:bg-gray-600',
                                'inline-flex items-center gap-2 px-4 py-2 border border-gray-600 rounded-md shadow-sm text-sm font-medium focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500'
                            ]"
                        >
                            <Settings class="h-4 w-4" />
                            Settings
                        </button>
                        <button
                            @click="activeTab = 'logs'"
                            :class="[
                                activeTab === 'logs'
                                    ? 'bg-indigo-600 text-white'
                                    : 'bg-gray-700 text-gray-200 hover:bg-gray-600',
                                'inline-flex items-center gap-2 px-4 py-2 border border-gray-600 rounded-md shadow-sm text-sm font-medium focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500'
                            ]"
                        >
                            <ScrollText class="h-4 w-4" />
                            Logs
                        </button>
                    </div>
                </div>
            </div>

            <div class="border-t border-gray-200 dark:border-gray-700">
                <div v-if="message" class="px-4 py-3 bg-green-50 dark:bg-green-900/20 border-b border-green-200 dark:border-green-800">
                    <p class="text-sm text-green-800 dark:text-green-300">{{ message }}</p>
                </div>

                <div v-if="error" class="px-4 py-3 bg-red-50 dark:bg-red-900/20 border-b border-red-200 dark:border-red-800">
                    <p class="text-sm text-red-800 dark:text-red-300">{{ error }}</p>
                </div>

                <!-- Settings Tab -->
                <div v-if="activeTab === 'settings'" class="px-4 py-5 sm:p-6">
                    <div class="flex gap-2 mb-6 flex-wrap">
                        <button
                            @click="testConnection"
                            :disabled="testing"
                            class="inline-flex items-center gap-2 px-4 py-2 border border-gray-600 rounded-md shadow-sm text-sm font-medium text-gray-200 bg-gray-700 hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50"
                        >
                            <Zap class="h-4 w-4" />
                            {{ testing ? 'Testing...' : 'Test Connectivity' }}
                        </button>
                        <button
                            @click="importFromConfig"
                            class="inline-flex items-center gap-2 px-4 py-2 border border-gray-600 rounded-md shadow-sm text-sm font-medium text-gray-200 bg-gray-700 hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
                        >
                            <Download class="h-4 w-4" />
                            Import from Config
                        </button>
                        <button
                            @click="exportToConfig"
                            class="inline-flex items-center gap-2 px-4 py-2 border border-gray-600 rounded-md shadow-sm text-sm font-medium text-gray-200 bg-gray-700 hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
                        >
                            <Upload class="h-4 w-4" />
                            Export to Config File
                        </button>
                        <button
                            @click="saveAllSettings"
                            :disabled="saving"
                            class="inline-flex items-center gap-2 px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50"
                        >
                            <Save class="h-4 w-4" />
                            {{ saving ? 'Saving...' : 'Save All Changes' }}
                        </button>
                    </div>

                    <!-- Test Results -->
                    <div v-if="testResults.length > 0" class="mb-6">
                        <div class="rounded-md bg-blue-900/20 border border-blue-700 p-4">
                            <div class="flex">
                                <div class="flex-shrink-0">
                                    <svg class="h-5 w-5 text-blue-400" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
                                        <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clip-rule="evenodd" />
                                    </svg>
                                </div>
                                <div class="ml-3 flex-1">
                                    <h3 class="text-sm font-medium text-blue-300">Connectivity Test Results</h3>
                                    <div class="mt-2 text-sm text-blue-200">
                                        <ul class="list-disc pl-5 space-y-1">
                                            <li v-for="result in testResults" :key="result.name">
                                                <span class="font-medium">{{ result.name }}:</span>
                                                <span :class="{
                                                    'text-green-600': result.status === 'success',
                                                    'text-red-600': result.status === 'failed' || result.status === 'error'
                                                }">
                                                    {{ result.message }}
                                                </span>
                                            </li>
                                        </ul>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>

                    <div v-for="(groupSettings, groupName) in localSettings" :key="groupName" class="mb-8">
                        <button
                            @click="toggleSection(groupName)"
                            class="w-full flex items-center justify-between gap-2 mb-4 pb-2 border-b border-gray-700 hover:border-indigo-500 transition-colors cursor-pointer"
                        >
                            <div class="flex items-center gap-2">
                                <component :is="getGroupIcon(groupName)" class="h-5 w-5 text-indigo-400" />
                                <h4 class="text-md font-semibold text-gray-100">
                                    {{ formatGroupName(groupName) }} Settings
                                </h4>
                            </div>
                            <ChevronDown
                                v-if="expandedSections[groupName]"
                                class="h-5 w-5 text-gray-400"
                            />
                            <ChevronUp
                                v-else
                                class="h-5 w-5 text-gray-400"
                            />
                        </button>
                        <div v-show="expandedSections[groupName]" class="space-y-6">
                            <div
                                v-for="setting in groupSettings"
                                :key="setting.id"
                            >
                                <Checkbox
                                    v-if="setting.type === 'boolean'"
                                    v-model="setting.actual_value"
                                    :label="setting.label || formatKey(setting.key)"
                                    :description="setting.description"
                                    :required="setting.is_sensitive"
                                />
                                <TextArea
                                    v-else-if="setting.type === 'json'"
                                    v-model="setting.actual_value"
                                    :label="setting.label || formatKey(setting.key)"
                                    :description="setting.description"
                                    :placeholder="setting.is_sensitive ? '********' : 'Enter JSON value'"
                                    :rows="4"
                                    :required="setting.is_sensitive"
                                />
                                <TextInput
                                    v-else
                                    v-model="setting.actual_value"
                                    :label="setting.label || formatKey(setting.key)"
                                    :description="setting.description"
                                    :type="setting.is_sensitive ? 'password' : 'text'"
                                    :placeholder="setting.is_sensitive ? '********' : 'Enter value'"
                                    :required="setting.is_sensitive"
                                />
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Logs Tab -->
                <div v-if="activeTab === 'logs'" class="px-4 py-5 sm:p-6">
                    <div class="mb-4 grid grid-cols-1 gap-4">
                        <div @keyup.enter="fetchLogs">
                            <TextInput
                                v-model="logSearch"
                                placeholder="Search logs..."
                                label="Search Logs"
                            />
                        </div>
                        <div class="flex gap-2 flex-wrap">
                            <button
                                @click="fetchLogs"
                                :disabled="loadingLogs"
                                class="inline-flex items-center gap-2 px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50"
                            >
                                <Search class="h-4 w-4" />
                                {{ loadingLogs ? 'Loading...' : 'Search' }}
                            </button>
                            <button
                                @click="logSearch = ''; fetchLogs()"
                                class="inline-flex items-center gap-2 px-4 py-2 border border-gray-600 rounded-md shadow-sm text-sm font-medium text-gray-200 bg-gray-700 hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
                            >
                                <X class="h-4 w-4" />
                                Clear
                            </button>
                            <a
                                href="/agent-settings/logs/download"
                                class="inline-flex items-center gap-2 px-4 py-2 border border-gray-600 rounded-md shadow-sm text-sm font-medium text-gray-200 bg-gray-700 hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
                            >
                                <Download class="h-4 w-4" />
                                Download Entire Log
                            </a>
                        </div>
                    </div>

                    <div class="mb-2 flex items-center gap-2">
                        <span v-if="logFileName" class="text-sm text-gray-500 dark:text-gray-400">
                            Showing: {{ logFileName }}
                        </span>
                        <Loader2
                            v-if="loadingLogs"
                            class="h-4 w-4 text-indigo-600 dark:text-indigo-400 animate-spin"
                        />
                        <span v-if="!loadingLogs" class="text-xs text-green-600 dark:text-green-400">● Live</span>
                    </div>

                    <div class="bg-gray-900 rounded-lg p-4 overflow-auto" style="max-height: 600px;">
                        <div v-if="loadingLogs && !logContent" class="flex items-center justify-center py-12">
                            <Loader2 class="h-8 w-8 text-green-400 animate-spin" />
                        </div>
                        <pre v-else class="text-xs text-green-400 font-mono whitespace-pre-wrap">{{ logContent || 'No logs to display' }}</pre>
                    </div>
                </div>
            </div>
        </div>
    </AppLayout>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onUnmounted, watch } from 'vue';
import { router } from '@inertiajs/vue3';
import AppLayout from '@/Layouts/AppLayout.vue';
import TextInput from '@/Components/Forms/TextInput.vue';
import Checkbox from '@/Components/Forms/Checkbox.vue';
import TextArea from '@/Components/Forms/TextArea.vue';
import {
    Cloud,
    HardDrive,
    FileText,
    Printer,
    Network,
    GraduationCap,
    Users,
    Loader2,
    Settings,
    ScrollText,
    ChevronDown,
    ChevronUp,
    Zap,
    Download,
    Upload,
    Save,
    Search,
    X
} from 'lucide-vue-next';

const props = defineProps({
    settings: Object,
    groups: Array,
});

const localSettings = reactive({ ...props.settings });
const saving = ref(false);
const message = ref('');
const error = ref('');

// Tab management
const activeTab = ref('settings');

// Test connectivity
const testing = ref(false);
const testResults = ref([]);

// Logs
const loadingLogs = ref(false);
const logContent = ref('');
const logFileName = ref('');
const logSearch = ref('');
let logPollingInterval = null;

// Collapse/expand sections - all expanded by default
const expandedSections = ref({});

// Initialize all sections as expanded
Object.keys(props.settings).forEach(groupName => {
    expandedSections.value[groupName] = true;
});

// Map group names to icons
const groupIcons = {
    tenant: Cloud,
    device_manager: HardDrive,
    logging: FileText,
    papercut: Printer,
    proxies: Network,
    emc: GraduationCap,
    ldap: Users,
};

function getGroupIcon(groupName) {
    return groupIcons[groupName] || FileText;
}

function formatGroupName(groupName) {
    return groupName.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase());
}

function formatKey(key) {
    const parts = key.split('.');
    const lastPart = parts[parts.length - 1];
    return lastPart.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase());
}

function toggleSection(groupName) {
    expandedSections.value[groupName] = !expandedSections.value[groupName];
}

async function saveAllSettings() {
    saving.value = true;
    message.value = '';
    error.value = '';

    const settingsToUpdate = [];
    Object.values(localSettings).forEach(group => {
        group.forEach(setting => {
            settingsToUpdate.push({
                id: setting.id,
                value: setting.actual_value,
            });
        });
    });

    try {
        const response = await fetch('/agent-settings', {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json',
                'X-CSRF-TOKEN': document.querySelector('meta[name="csrf-token"]')?.content,
            },
            body: JSON.stringify({ settings: settingsToUpdate }),
        });

        const data = await response.json();

        if (response.ok) {
            message.value = 'Settings saved successfully!';
            setTimeout(() => {
                message.value = '';
            }, 3000);
        } else {
            error.value = data.message || 'Failed to save settings';
        }
    } catch (err) {
        error.value = 'An error occurred while saving settings';
    } finally {
        saving.value = false;
    }
}

async function importFromConfig() {
    try {
        const response = await fetch('/agent-settings/import', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-CSRF-TOKEN': document.querySelector('meta[name="csrf-token"]')?.content,
            },
        });

        const data = await response.json();

        if (response.ok) {
            message.value = data.message;
            setTimeout(() => {
                router.reload();
            }, 1000);
        } else {
            error.value = data.message || 'Failed to import settings';
        }
    } catch (err) {
        error.value = 'An error occurred while importing settings';
    }
}

async function exportToConfig() {
    try {
        const response = await fetch('/agent-settings/export', {
            method: 'GET',
            headers: {
                'X-CSRF-TOKEN': document.querySelector('meta[name="csrf-token"]')?.content,
            },
        });

        const data = await response.json();

        if (response.ok) {
            message.value = data.message;
            setTimeout(() => {
                message.value = '';
            }, 3000);
        } else {
            error.value = data.message || 'Failed to export settings';
        }
    } catch (err) {
        error.value = 'An error occurred while exporting settings';
    }
}

async function testConnection() {
    testing.value = true;
    testResults.value = [];
    message.value = '';
    error.value = '';

    try {
        const response = await fetch('/agent-settings/test-connection', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-CSRF-TOKEN': document.querySelector('meta[name="csrf-token"]')?.content,
            },
        });

        const data = await response.json();

        if (response.ok) {
            testResults.value = data.results || [];
            if (data.success) {
                message.value = data.message;
            } else {
                error.value = data.message;
            }
            setTimeout(() => {
                message.value = '';
                error.value = '';
            }, 5000);
        } else {
            error.value = data.message || 'Failed to test connection';
        }
    } catch (err) {
        error.value = 'An error occurred while testing connection';
    } finally {
        testing.value = false;
    }
}

async function fetchLogs(silent = false) {
    if (!silent) {
        loadingLogs.value = true;
    }
    error.value = '';

    try {
        const params = new URLSearchParams();
        if (logSearch.value) {
            params.append('search', logSearch.value);
        }

        const response = await fetch(`/agent-settings/logs?${params}`, {
            method: 'GET',
            headers: {
                'X-CSRF-TOKEN': document.querySelector('meta[name="csrf-token"]')?.content,
            },
        });

        const data = await response.json();

        if (response.ok) {
            logContent.value = data.content;
            logFileName.value = data.file;
        } else {
            if (!silent) {
                error.value = 'Failed to fetch logs';
            }
        }
    } catch (err) {
        if (!silent) {
            error.value = 'An error occurred while fetching logs';
        }
    } finally {
        loadingLogs.value = false;
    }
}

function startLogPolling() {
    // Clear any existing interval
    stopLogPolling();

    // Start polling every 3 seconds
    logPollingInterval = setInterval(() => {
        fetchLogs(true); // Silent fetch for polling
    }, 3000);
}

function stopLogPolling() {
    if (logPollingInterval) {
        clearInterval(logPollingInterval);
        logPollingInterval = null;
    }
}

// Watch for tab changes and start/stop polling
watch(activeTab, (newTab) => {
    if (newTab === 'logs') {
        fetchLogs();
        startLogPolling();
    } else {
        stopLogPolling();
    }
});

// Start polling on mount if on logs tab
onMounted(() => {
    if (activeTab.value === 'logs') {
        fetchLogs();
        startLogPolling();
    }
});

// Clean up polling on unmount
onUnmounted(() => {
    stopLogPolling();
});
</script>
