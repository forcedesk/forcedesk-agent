<?php

namespace App\Services;

use Laravel\Dusk\Browser;
use Laravel\Dusk\Chrome;
use Facebook\WebDriver\Chrome\ChromeOptions;
use Facebook\WebDriver\WebDriverDimension;
use Illuminate\Support\Facades\Http;
use Illuminate\Http\Client\Response;
use Illuminate\Support\Facades\Log;
use Exception;

class EdustarHybridService
{
    private $session;
    private $connection;
    private $headers;
    private $browser;
    private $baseUrl;
    private $timeout;

    public function __construct()
    {
        $this->session = [];
        $this->connection = null;
        $this->browser = null;
        $this->baseUrl = config('services.edustar.base_url', 'https://apps.edustar.vic.edu.au');
        $this->timeout = config('services.edustar.timeout', 30);

        $this->headers = [
            'User-Agent' => 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
            'Accept' => 'application/json, text/html, application/xhtml+xml, application/xml;q=0.9, */*;q=0.8',
            'Accept-Language' => 'en-US,en;q=0.9',
            'Accept-Encoding' => 'gzip, deflate, br',
            'Cache-Control' => 'no-cache',
            'Pragma' => 'no-cache',
        ];
    }

    /**
     * Connect using browser automation, then extract session for HTTP calls
     */
    public function connect(string $username, string $password): array
    {
        Log::info("Starting hybrid EduSTAR authentication...");

        try {
            // Step 1: Browser authentication
            $this->authenticateWithBrowser($username, $password);

            // Step 2: Extract cookies from browser
            $this->extractSessionFromBrowser();

            // Step 3: Test HTTP access
            $this->testHttpAccess();

            // Step 4: Get user info if possible
            $userInfo = $this->getUserInfo();

            $connectionInfo = [
                'connected' => true,
                'logged_in_as' => $userInfo['name'] ?? 'Authenticated User',
                'user_details' => $userInfo,
                'schools' => $userInfo['schools'] ?? 1,
                'session_cookies' => count($this->session),
                'authentication_method' => 'hybrid_browser_http'
            ];

            $this->connection = $connectionInfo;

            Log::info("✅ Hybrid authentication successful with " . count($this->session) . " cookies");

            return $connectionInfo;

        } catch (Exception $e) {
            $this->cleanup();
            Log::error("❌ Hybrid authentication failed: " . $e->getMessage());
            throw $e;
        }
    }

    /**
     * Authenticate using browser automation
     */
    private function authenticateWithBrowser(string $username, string $password): void
    {
        Log::info("🌐 Starting browser authentication...");

        // Configure Chrome options for server environment
        $options = (new ChromeOptions())->addArguments([
            '--no-sandbox',
            '--disable-dev-shm-usage',
            '--disable-gpu',
            '--disable-web-security',
            '--disable-features=VizDisplayCompositor',
            '--disable-extensions',
            '--disable-plugins',
            '--disable-images',
            '--disable-javascript-harmony-shipping',
            '--disable-background-timer-throttling',
            '--disable-renderer-backgrounding',
            '--disable-backgrounding-occluded-windows',
            '--disable-ipc-flooding-protection',
            '--window-size=1920,1080',
        ]);

        // Add headless mode based on environment
        if (config('dusk.headless', true)) {
            $options->addArguments(['--headless']);
        }

        try {
            $driver = Chrome::driver(null, $options);
            $this->browser = new Browser($driver);

            // Set browser window size
            $this->browser->driver->manage()->window()->setSize(new WebDriverDimension(1920, 1080));

            // Navigate to login page with retries
            $this->navigateToLogin();

            // Perform login
            $this->performLogin($username, $password);

            // Verify authentication
            $this->verifyAuthentication();

            Log::info("✅ Browser authentication completed successfully");

        } catch (Exception $e) {
            Log::error("❌ Browser authentication failed: " . $e->getMessage());
            throw $e;
        }
    }

    private function navigateToLogin(): void
    {
        $loginUrl = $this->baseUrl . '/my.policy';
        $maxRetries = 3;

        for ($i = 0; $i < $maxRetries; $i++) {
            try {
                Log::info("Navigating to login page (attempt " . ($i + 1) . "): {$loginUrl}");

                $this->browser->visit($loginUrl);
                $this->browser->pause(3000); // Wait for page load

                $currentUrl = $this->browser->driver->getCurrentURL();
                Log::info("Current URL: {$currentUrl}");

                // Check if we're on a login-related page
                if (str_contains($currentUrl, 'my.policy') ||
                    str_contains($currentUrl, 'login') ||
                    $this->browser->element('[name="username"]') ||
                    $this->browser->element('[name="password"]')) {

                    Log::info("✅ Successfully reached login page");
                    return;
                }

                if ($i < $maxRetries - 1) {
                    Log::warning("Login page not detected, retrying...");
                    $this->browser->pause(2000);
                }

            } catch (Exception $e) {
                if ($i == $maxRetries - 1) {
                    throw new Exception("Failed to navigate to login page after {$maxRetries} attempts: " . $e->getMessage());
                }
                Log::warning("Navigation attempt " . ($i + 1) . " failed: " . $e->getMessage());
                $this->browser->pause(2000);
            }
        }

        throw new Exception("Could not reach login page after {$maxRetries} attempts");
    }

    private function performLogin(string $username, string $password): void
    {
        Log::info("📝 Filling login form...");

        try {
            // Wait for form elements
            $this->browser->waitFor('[name="username"]', 10);
            $this->browser->waitFor('[name="password"]', 10);

            // Clear and fill username
            $this->browser->clear('username')->type('username', $username);
            $this->browser->pause(500);

            // Clear and fill password
            $this->browser->clear('password')->type('password', $password);
            $this->browser->pause(500);

            Log::info("🚀 Submitting login form...");

            // Try multiple submit methods
            if ($this->browser->element('button[type="submit"]')) {
                $this->browser->click('button[type="submit"]');
            } elseif ($this->browser->element('input[type="submit"]')) {
                $this->browser->click('input[type="submit"]');
            } elseif ($this->browser->element('button:contains("Log")')) {
                $this->browser->clickLink('Log');
            } elseif ($this->browser->element('form')) {
                // Submit the form directly
                $this->browser->script('document.querySelector("form").submit();');
            } else {
                // Last resort: press Enter on password field
                $this->browser->keys('password', '{enter}');
            }

            // Wait for form submission
            $this->browser->pause(5000);

        } catch (Exception $e) {
            throw new Exception("Login form interaction failed: " . $e->getMessage());
        }
    }

    private function verifyAuthentication(): void
    {
        Log::info("🔍 Verifying authentication...");

        $currentUrl = $this->browser->driver->getCurrentURL();
        Log::info("Post-login URL: {$currentUrl}");

        $maxWait = 15; // seconds
        $waited = 0;

        while ($waited < $maxWait) {
            $currentUrl = $this->browser->driver->getCurrentURL();

            // Check for successful authentication indicators
            if (str_contains($currentUrl, 'edustarmc')) {
                Log::info("✅ Authentication successful - redirected to edustarmc");
                return;
            }

            // Check if still on login page
            if (str_contains($currentUrl, 'my.policy')) {
                $pageSource = $this->browser->driver->getPageSource();

                // Look for error messages
                if (str_contains($pageSource, 'Invalid') ||
                    str_contains($pageSource, 'error') ||
                    str_contains($pageSource, 'denied') ||
                    str_contains($pageSource, 'incorrect')) {
                    throw new Exception("Authentication failed - invalid credentials or access denied");
                }

                // Still processing, wait more
                if ($waited < $maxWait - 5) {
                    $this->browser->pause(2000);
                    $waited += 2;
                    continue;
                }
            }

            // Check for any other redirect
            if (!str_contains($currentUrl, 'my.policy')) {
                Log::info("✅ Authentication appears successful - redirected to: {$currentUrl}");
                return;
            }

            $this->browser->pause(1000);
            $waited += 1;
        }

        throw new Exception("Authentication verification timeout - unclear if login was successful");
    }

    /**
     * Extract session cookies from browser
     */
    private function extractSessionFromBrowser(): void
    {
        Log::info("🍪 Extracting session cookies from browser...");

        if (!$this->browser) {
            throw new Exception("No browser session available");
        }

        $cookies = $this->browser->driver->manage()->getCookies();
        $extractedCount = 0;

        foreach ($cookies as $cookie) {
            $name = $cookie['name'];
            $value = $cookie['value'];
            $domain = $cookie['domain'] ?? '';

            // Keep cookies from edustar domain or session-related cookies
            if (str_contains($domain, 'edustar.vic.edu.au') ||
                empty($domain) ||
                str_contains($name, 'Session') ||
                str_contains($name, 'F5') ||
                str_contains($name, 'MRH') ||
                str_contains($name, 'TIN')) {

                $this->session[$name] = $value;
                $extractedCount++;
                Log::debug("Extracted cookie: {$name} (domain: {$domain})");
            }
        }

        Log::info("✅ Extracted {$extractedCount} session cookies");

        if ($extractedCount === 0) {
            throw new Exception("No session cookies extracted - authentication may have failed");
        }
    }

    /**
     * Test HTTP access with extracted cookies
     */
    private function testHttpAccess(): void
    {
        Log::info("🌐 Testing HTTP access with extracted cookies...");

        $headers = array_merge($this->headers, [
            'Cookie' => $this->formatCookies($this->session),
            'Referer' => $this->baseUrl . '/my.policy'
        ]);

        $response = Http::withHeaders($headers)
            ->withOptions([
                'timeout' => $this->timeout,
                'verify' => false
            ])
            ->get($this->baseUrl . '/edustarmc/');

        Log::debug("HTTP test response status: " . $response->status());

        if (!$response->successful()) {
            throw new Exception("HTTP access test failed with status: " . $response->status());
        }

        $responseBody = $response->body();

        // Check for login form indicators
        if ((str_contains($responseBody, 'username') && str_contains($responseBody, 'password')) ||
            str_contains($responseBody, '"pageType": "logon"') ||
            str_contains($responseBody, 'auth_form')) {
            throw new Exception("HTTP test failed - response contains login form, session may be invalid");
        }

        Log::info("✅ HTTP access test successful");
    }

    /**
     * Get user information
     */
    private function getUserInfo(): array
    {
        try {
            // Try to get user info from the main page
            $headers = array_merge($this->headers, [
                'Cookie' => $this->formatCookies($this->session),
                'Referer' => $this->baseUrl . '/edustarmc/'
            ]);

            $response = Http::withHeaders($headers)
                ->withOptions([
                    'timeout' => $this->timeout,
                    'verify' => false
                ])
                ->get($this->baseUrl . '/edustarmc/');

            if ($response->successful()) {
                $content = $response->body();

                // Try to extract user info from page content
                $userInfo = [
                    'name' => $this->extractUserName($content),
                    'schools' => $this->extractSchoolCount($content)
                ];

                return $userInfo;
            }
        } catch (Exception $e) {
            Log::debug("Could not get user info: " . $e->getMessage());
        }

        return ['name' => 'Authenticated User', 'schools' => 1];
    }

    private function extractUserName(string $content): string
    {
        $patterns = [
            '/Welcome,?\s+([^<,\n]+)/i',
            '/Hello,?\s+([^<,\n]+)/i',
            '/Hi,?\s+([^<,\n]+)/i',
            '/<span[^>]*class="[^"]*user[^"]*"[^>]*>([^<]+)</i',
            '/<div[^>]*class="[^"]*profile[^"]*"[^>]*>([^<]+)</i'
        ];

        foreach ($patterns as $pattern) {
            if (preg_match($pattern, $content, $matches)) {
                return trim($matches[1]);
            }
        }

        return 'Unknown User';
    }

    private function extractSchoolCount(string $content): int
    {
        if (preg_match_all('/school/i', $content, $matches)) {
            return min(count($matches[0]), 10); // Cap at reasonable number
        }

        return 1;
    }

    /**
     * Make HTTP API calls using extracted session cookies
     */
    public function makeApiCall(string $endpoint, string $method = 'GET', array $data = []): Response
    {
        if (!$this->isConnected()) {
            throw new Exception('Not connected to eduSTAR. Please authenticate first.');
        }

        Log::info("🔗 Making {$method} API call to: {$endpoint}");

        $headers = array_merge($this->headers, [
            'Cookie' => $this->formatCookies($this->session),
            'Referer' => $this->baseUrl . '/edustarmc/',
            'X-Requested-With' => 'XMLHttpRequest'
        ]);

        if (in_array(strtoupper($method), ['POST', 'PUT', 'PATCH']) && !empty($data)) {
            $headers['Content-Type'] = 'application/json';
        }

        $http = Http::withHeaders($headers)
            ->withOptions([
                'timeout' => $this->timeout,
                'verify' => false,
                'allow_redirects' => false
            ]);

        try {
            switch (strtoupper($method)) {
                case 'POST':
                    $response = $http->post($endpoint, $data);
                    break;
                case 'PUT':
                    $response = $http->put($endpoint, $data);
                    break;
                case 'DELETE':
                    $response = $http->delete($endpoint);
                    break;
                case 'PATCH':
                    $response = $http->patch($endpoint, $data);
                    break;
                default:
                    $response = $http->get($endpoint);
                    break;
            }

            Log::debug("API response status: " . $response->status());

            // Check for session expiration
            if ($response->status() >= 300 && $response->status() < 400) {
                $location = $response->header('Location');
                if ($location && (str_contains($location, 'my.policy') || str_contains($location, 'login'))) {
                    throw new Exception('Session expired - API redirected to login');
                }
            }

            // Check response content for login indicators
            $responseBody = $response->body();
            if (str_contains($responseBody, '"pageType": "logon"') ||
                str_contains($responseBody, 'auth_form')) {
                throw new Exception('Session expired - API response contains login form');
            }

            return $response;

        } catch (Exception $e) {
            Log::error("API call failed: " . $e->getMessage());
            throw $e;
        }
    }

    /**
     * Make browser-based API calls (fallback)
     */
    public function makeBrowserApiCall(string $endpoint): string
    {
        if (!$this->browser) {
            throw new Exception('Browser session not available');
        }

        Log::info("🌐 Making browser API call to: {$endpoint}");

        try {
            $this->browser->visit($endpoint)->pause(3000);
            return $this->browser->driver->getPageSource();
        } catch (Exception $e) {
            Log::error("Browser API call failed: " . $e->getMessage());
            throw $e;
        }
    }

    /**
     * Get students data
     */
    public function getStudents(string $schoolNumber, bool $useBrowser = false): Response|string
    {
        $endpoint = "{$this->baseUrl}/edustarmc/api/MC/GetStudents/{$schoolNumber}/FULL";

        if ($useBrowser) {
            return $this->makeBrowserApiCall($endpoint);
        } else {
            return $this->makeApiCall($endpoint);
        }
    }

    /**
     * Refresh session using browser
     */
    public function refreshSession(): void
    {
        if (!$this->browser) {
            throw new Exception('Browser session not available for refresh');
        }

        Log::info("🔄 Refreshing session...");

        try {
            $this->browser->visit($this->baseUrl . '/edustarmc/')->pause(3000);
            $this->extractSessionFromBrowser();
            Log::info("✅ Session refreshed successfully");
        } catch (Exception $e) {
            Log::error("Session refresh failed: " . $e->getMessage());
            throw $e;
        }
    }

    /**
     * Utility methods
     */
    private function formatCookies(array $cookies): string
    {
        if (empty($cookies)) {
            return '';
        }

        $cookiePairs = [];
        foreach ($cookies as $name => $value) {
            $cookiePairs[] = "{$name}={$value}";
        }
        return implode('; ', $cookiePairs);
    }

    public function isConnected(): bool
    {
        return $this->connection && $this->connection['connected'];
    }

    public function getConnection(): ?array
    {
        return $this->connection;
    }

    public function getSessionCookies(): array
    {
        return $this->session;
    }

    /**
     * Clean up resources
     */
    public function cleanup(): void
    {
        if ($this->browser) {
            try {
                $this->browser->quit();
                Log::info("🧹 Browser session cleaned up");
            } catch (Exception $e) {
                Log::warning("Browser cleanup warning: " . $e->getMessage());
            }
            $this->browser = null;
        }
    }

    public function __destruct()
    {
        $this->cleanup();
    }
}
