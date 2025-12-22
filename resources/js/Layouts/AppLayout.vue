<template>
    <div class="min-h-screen bg-gray-100 dark:bg-gray-900 flex flex-col">
        <nav class="bg-white dark:bg-gray-800 shadow-md dark:shadow-gray-900/50 border-b border-gray-200 dark:border-gray-700">
            <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
                <div class="flex justify-between h-16">
                    <div class="flex items-center gap-3">
                        <img
                            src="https://cdn.forcedesk.io/img/forcedesk-icon-test.svg"
                            alt="ForceDesk Logo"
                            class="h-8 w-8"
                        />
                        <h1 class="text-xl font-semibold text-gray-900 dark:text-white">
                            ForceDeskAgent Configuration
                        </h1>
                    </div>
                    <div class="flex items-center">
                        <button
                            @click="handleLogout"
                            class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-gray-700 dark:text-gray-200 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-gray-500 dark:focus:ring-gray-400"
                        >
                            <LogOut class="h-4 w-4 mr-2" />
                            Logout
                        </button>
                    </div>
                </div>
            </div>
        </nav>

        <main class="flex-1 py-10">
            <div class="max-w-7xl mx-auto sm:px-6 lg:px-8">
                <slot />
            </div>
        </main>

        <!-- Footer -->
        <footer class="bg-white dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700 mt-auto">
            <div class="mt-4 sm:mt-8 py-4 sm:py-6 text-center text-gray-500 dark:text-gray-400 border-t border-gray-200 dark:border-gray-700">
                <div class="flex flex-col items-center px-3">
                    <!-- Footer Links -->
                    <div class="flex flex-wrap items-center justify-center gap-3 sm:gap-6 mb-4 text-xs sm:text-sm">
                        <Link
                            href="/privacy-policy"
                            class="text-gray-600 dark:text-gray-400 hover:text-blue-600 dark:hover:text-blue-400 transition-colors flex items-center gap-1.5"
                        >
                            <ShieldCheck class="w-3.5 h-3.5" />
                            Privacy Policy
                        </Link>
                        <Dot class="w-3 h-3 text-gray-400 dark:text-gray-600" />
                        <Link
                            href="/terms-of-service"
                            class="text-gray-600 dark:text-gray-400 hover:text-blue-600 dark:hover:text-blue-400 transition-colors flex items-center gap-1.5"
                        >
                            <FileText class="w-3.5 h-3.5" />
                            Terms of Service
                        </Link>
                        <Dot class="w-3 h-3 text-gray-400 dark:text-gray-600" />
                        <a
                            href="https://docs.forcedesk.io"
                            target="_blank"
                            rel="noopener noreferrer"
                            class="text-gray-600 dark:text-gray-400 hover:text-blue-600 dark:hover:text-blue-400 transition-colors flex items-center gap-1.5"
                        >
                            <BookOpen class="w-3.5 h-3.5" />
                            Documentation
                        </a>
                        <Dot class="w-3 h-3 text-gray-400 dark:text-gray-600" />
                        <a
                            href="https://forcedesk.io/status"
                            target="_blank"
                            rel="noopener noreferrer"
                            class="text-gray-600 dark:text-gray-400 hover:text-blue-600 dark:hover:text-blue-400 transition-colors flex items-center gap-1.5"
                        >
                            <Activity class="w-3.5 h-3.5" />
                            Service Status
                        </a>
                        <Dot class="w-3 h-3 text-gray-400 dark:text-gray-600" />
                        <a
                            href="https://github.com/forcedesk/forcedesk/issues"
                            target="_blank"
                            rel="noopener noreferrer"
                            class="text-gray-600 dark:text-gray-400 hover:text-blue-600 dark:hover:text-blue-400 transition-colors flex items-center gap-1.5"
                        >
                            <Bug class="w-3.5 h-3.5" />
                            Report a Bug
                        </a>
                    </div>

                    <!-- Powered By -->
                    <span class="text-xs mb-1">Powered by</span>
                    <a href="https://www.forcedesk.io" target="_blank" rel="nofollow" class="mb-2">
                        <img v-if="isDark" src="https://cdn.forcedesk.io/img/forcedesk-light-test.svg" alt="ForceDesk" class="h-5 sm:h-6" />
                        <img v-else src="https://cdn.forcedesk.io/img/forcedesk-footer-test.svg" alt="ForceDesk" class="h-5 sm:h-6" />
                    </a>
                    <span class="text-xs text-gray-400 dark:text-gray-500">Copyright &copy; ForcePoint Software</span>
                </div>
            </div>
        </footer>

        <!-- Logout Confirmation Dialog -->
        <ConfirmDialog
            :show="showLogoutDialog"
            title="Confirm Logout"
            message="Are you sure you want to logout?"
            confirmText="Logout"
            cancelText="Cancel"
            type="danger"
            @confirm="confirmLogout"
            @cancel="showLogoutDialog = false"
        />
    </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue';
import { router, Link } from '@inertiajs/vue3';
import { LogOut, ShieldCheck, FileText, BookOpen, Activity, Bug, Dot } from 'lucide-vue-next';
import ConfirmDialog from '@/Components/ConfirmDialog.vue';

const showLogoutDialog = ref(false);
const isDark = ref(false);

// Detect dark mode
onMounted(() => {
    const darkModeMediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    isDark.value = darkModeMediaQuery.matches;

    darkModeMediaQuery.addEventListener('change', (e) => {
        isDark.value = e.matches;
    });
});

function handleLogout() {
    showLogoutDialog.value = true;
}

function confirmLogout() {
    showLogoutDialog.value = false;
    router.post('/logout');
}
</script>
