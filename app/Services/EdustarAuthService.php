<?php

namespace App\Services;

use Illuminate\Http\Client\Response;
use Illuminate\Support\Facades\Http;
use Illuminate\Support\Facades\Log;
use Exception;

class EdustarAuthService
{
    private $maxAttempts;
    private $session;
    private $connection;
    private $headers;

    public function __construct(int $maxAttempts = 3)
    {
        $this->maxAttempts = $maxAttempts;
        $this->session = null;
        $this->connection = null;
        $this->headers = [
            'User-Agent' => 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
            'Accept' => 'application/json, text/plain, */*',
            'Accept-Language' => 'en-US,en;q=0.9',
            'Referer' => 'https://apps.edustar.vic.edu.au/edustarmc/',
        ];
    }

    /**
     * Connect to eduSTAR with credentials
     *
     * @param string $username
     * @param string $password
     * @return array
     * @throws Exception
     */
    public function connect(string $username, string $password): array
    {
        $success = false;
        $attempt = 0;

        while (!$success && $attempt < $this->maxAttempts) {
            $attempt++;
            Log::info("Connection attempt {$attempt} of {$this->maxAttempts}...");

            try {
                $this->session = null;

                Log::info("Getting login page...");
                $loginPageResponse = Http::withOptions([
                    'allow_redirects' => ['max' => 5],
                    'timeout' => 30,
                ])->get('https://apps.edustar.vic.edu.au/edustarmc/');

                if (!$loginPageResponse->successful()) {
                    throw new Exception("Failed to get login page: " . $loginPageResponse->status());
                }

                $loginContent = $loginPageResponse->body();
                $cookies = $this->extractCookies($loginPageResponse);

                if (strpos($loginContent, 'Access policy evaluation is already in progress') !== false) {
                    Log::info("Detected session conflict, extracting newsession URI...");

                    $newSessionUri = $this->extractNewSessionUri($loginContent);

                    if ($newSessionUri) {
                        Log::info("Found newsession URI: {$newSessionUri}");

                        $newSessionUrl = "https://apps.edustar.vic.edu.au/logon.php3?{$newSessionUri}";
                        Log::info("Creating new session via: {$newSessionUrl}");

                        $cleanResponse = Http::withOptions([
                            'allow_redirects' => ['max' => 5],
                            'timeout' => 30,
                        ])->get($newSessionUrl);

                        if (!$cleanResponse->successful()) {
                            throw new Exception("Failed to create new session: " . $cleanResponse->status());
                        }

                        $cookies = $this->extractCookies($cleanResponse);

                        sleep(5);

                        Log::info("New session established, proceeding with authentication...");
                    } else {
                        Log::warning("Could not extract newsession URI, will try direct authentication...");
                    }
                } else {
                    Log::info("Login page accessible, proceeding with authentication...");
                }

                $authData = [
                    'curl' => 'Z2Fedustarmc',
                    'username' => $username,
                    'password' => $password,
                    'SubmitCreds' => 'Log+in'
                ];

                $authHeaders = array_merge($this->headers, [
                    'Accept' => 'text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8',
                    'Accept-Language' => 'en-US,en;q=0.5',
                    'Content-Type' => 'application/x-www-form-urlencoded',
                    'Cache-Control' => 'no-cache',
                    'Pragma' => 'no-cache',
                ]);

                if (!empty($cookies)) {
                    $authHeaders['Cookie'] = $this->formatCookies($cookies);
                }

                Log::info("Submitting credentials...");
                $authResponse = Http::withHeaders($authHeaders)
                    ->withOptions([
                        'allow_redirects' => ['max' => 5],
                        'timeout' => 30,
                    ])
                    ->asForm()
                    ->post('https://apps.edustar.vic.edu.au/my.policy', $authData);

                if (!$authResponse->successful()) {
                    throw new Exception("Authentication failed: " . $authResponse->status());
                }

                // Store session cookies for subsequent requests
                $this->session = $this->extractCookies($authResponse);

                Log::info("Testing API access...");
                $connectionTest = $this->testConnection();

                if (!$connectionTest['success']) {
                    throw new Exception("Connection test failed: " . $connectionTest['error']);
                }

                $getUser = $connectionTest['getUser'];

                $connectionInfo = [
                    'connected' => true,
                    'logged_in_as' => $getUser['User']['_displayName'] ?? 'Unknown',
                    'user_details' => $getUser['User'] ?? [],
                    'schools' => $getUser['User']['_schools']['ChildNodes']['Count'] ?? 0
                ];

                $this->connection = $connectionInfo;

                Log::info("Successfully connected as: " . $connectionInfo['logged_in_as']);
                Log::info("Access to " . $connectionInfo['schools'] . " school(s)");

                $success = true;
                return $connectionInfo;

            } catch (Exception $e) {
                Log::error("Attempt {$attempt} failed: " . $e->getMessage());

                if ($attempt < $this->maxAttempts) {
                    Log::info("Waiting before retry...");
                    sleep(10);
                } else {
                    Log::error("All connection attempts failed. The system may be experiencing issues.");

                    if ($this->session) {
                        Log::debug("Session cookies: " . $this->getMaskedCookiesString());
                    }

                    throw new Exception("All connection attempts failed. See debug information in logs.");
                }
            }
        }

        throw new Exception("Connection failed after {$this->maxAttempts} attempts");
    }

    /**
     * Test the connection to eduSTAR API
     *
     * @return array
     */
    private function testConnection(): array
    {
        try {
            $headers = $this->headers;
            if ($this->session) {
                $headers['Cookie'] = $this->formatCookies($this->session);
            }

            // This would be the actual API endpoint for testing connection
            $response = Http::withHeaders($headers)
                ->withOptions(['timeout' => 30])
                ->get('https://apps.edustar.vic.edu.au/edustarmc/api/user');

            if ($response->successful()) {
                return [
                    'success' => true,
                    'getUser' => $response->json()
                ];
            } else {
                return [
                    'success' => false,
                    'error' => 'API test failed: ' . $response->status()
                ];
            }
        } catch (Exception $e) {
            return [
                'success' => false,
                'error' => $e->getMessage()
            ];
        }
    }

    /**
     * Extract cookies from HTTP response
     *
     * @param Response $response
     * @return array
     */
    private function extractCookies(Response $response): array
    {
        $cookies = [];
        $setCookieHeaders = $response->header('Set-Cookie');

        if (is_string($setCookieHeaders)) {
            $setCookieHeaders = [$setCookieHeaders];
        }

        if (is_array($setCookieHeaders)) {
            foreach ($setCookieHeaders as $cookieHeader) {
                if (preg_match('/^([^=]+)=([^;]+)/', $cookieHeader, $matches)) {
                    $cookies[$matches[1]] = $matches[2];
                }
            }
        }

        return $cookies;
    }

    /**
     * Format cookies for HTTP header
     *
     * @param array $cookies
     * @return string
     */
    private function formatCookies(array $cookies): string
    {
        $cookiePairs = [];
        foreach ($cookies as $name => $value) {
            $cookiePairs[] = "{$name}={$value}";
        }
        return implode('; ', $cookiePairs);
    }

    /**
     * Extract new session URI from HTML content
     *
     * @param string $content
     * @return string|null
     */
    private function extractNewSessionUri(string $content): ?string
    {
        $pattern = '/"newsession",\s*"uri":\s*"([^"]*)"/';
        if (preg_match($pattern, $content, $matches)) {
            return $matches[1];
        }
        return null;
    }

    /**
     * Get masked cookies string for debugging
     *
     * @return string
     */
    private function getMaskedCookiesString(): string
    {
        if (!$this->session) {
            return 'No session cookies';
        }

        $masked = [];
        foreach ($this->session as $name => $value) {
            $maskedValue = strlen($value) > 8
                ? substr($value, 0, 4) . '****' . substr($value, -4)
                : '****';
            $masked[] = "{$name}={$maskedValue}";
        }

        return implode('; ', $masked);
    }

    /**
     * Get current connection info
     *
     * @return array|null
     */
    public function getConnection(): ?array
    {
        return $this->connection;
    }

    /**
     * Check if currently connected
     *
     * @return bool
     */
    public function isConnected(): bool
    {
        return $this->connection && $this->connection['connected'];
    }

    /**
     * Make authenticated API call
     *
     * @param string $endpoint
     * @param string $method
     * @param array $data
     * @return Response
     */
    public function makeApiCall(string $endpoint, string $method = 'GET', array $data = []): Response
    {
        if (!$this->isConnected()) {
            throw new Exception('Not connected to eduSTAR. Please authenticate first.');
        }

        $headers = $this->headers;
        if ($this->session) {
            $headers['Cookie'] = $this->formatCookies($this->session);
        }

        $http = Http::withHeaders($headers)->withOptions(['timeout' => 30]);

        switch (strtoupper($method)) {
            case 'POST':
                return $http->post($endpoint, $data);
            case 'PUT':
                return $http->put($endpoint, $data);
            case 'DELETE':
                return $http->delete($endpoint, $data);
            default:
                return $http->get($endpoint, $data);
        }
    }
}
