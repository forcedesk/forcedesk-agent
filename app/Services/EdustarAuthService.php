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
        $this->session = [];
        $this->connection = null;
        $this->headers = [
            'User-Agent' => 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:143.0) Gecko/20100101 Firefox/143.0',
            'Accept' => 'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
            'Accept-Language' => 'en-US,en;q=0.5',
            'Accept-Encoding' => 'gzip, deflate, br, zstd',
        ];
    }

    /**
     * Connect to eduSTAR with credentials
     */
    public function connect(string $username, string $password): array
    {
        $success = false;
        $attempt = 0;

        while (!$success && $attempt < $this->maxAttempts) {
            $attempt++;
            Log::info("Connection attempt {$attempt} of {$this->maxAttempts}...");

            try {
                $this->session = [];

                // Step 1: Get initial page to establish session
                Log::info("Getting initial login page...");
                $initialResponse = Http::withHeaders($this->headers)
                    ->withOptions([
                        'allow_redirects' => ['max' => 5],
                        'timeout' => 30,
                    ])
                    ->get('https://apps.edustar.vic.edu.au/my.policy');

                if (!$initialResponse->successful()) {
                    throw new Exception("Failed to get initial page: " . $initialResponse->status());
                }

                // Extract cookies from initial request
                $cookies = $this->extractCookies($initialResponse);
                Log::debug("Initial cookies: " . json_encode(array_keys($cookies)));

                // Step 2: Prepare authentication data - just username and password
                $authData = [
                    'username' => $username, // Should be in format like EDU001\\USERNAME
                    'password' => $password
                ];

                $authHeaders = array_merge($this->headers, [
                    'Content-Type' => 'application/x-www-form-urlencoded',
                    'Origin' => 'https://apps.edustar.vic.edu.au',
                    'Referer' => 'https://apps.edustar.vic.edu.au/my.policy',
                    'Connection' => 'keep-alive',
                    'Upgrade-Insecure-Requests' => '1',
                    'Sec-Fetch-Dest' => 'document',
                    'Sec-Fetch-Mode' => 'navigate',
                    'Sec-Fetch-Site' => 'same-origin',
                    'Sec-Fetch-User' => '?1',
                    'Priority' => 'u=0, i'
                ]);

                // Add cookies if we have them
                if (!empty($cookies)) {
                    $authHeaders['Cookie'] = $this->formatCookies($cookies);
                }

                Log::info("Submitting credentials to my.policy...");
                $authResponse = Http::withHeaders($authHeaders)
                    ->withOptions([
                        'allow_redirects' => false, // Don't follow redirects automatically
                        'timeout' => 30,
                    ])
                    ->asForm()
                    ->post('https://apps.edustar.vic.edu.au/my.policy', $authData);

                Log::debug("Auth response status: " . $authResponse->status());
                Log::debug("Auth response headers: " . json_encode($authResponse->headers()));

                // Should get 302 redirect on successful auth
                if ($authResponse->status() === 302) {
                    $redirectLocation = $authResponse->header('Location');
                    Log::info("Got redirect to: " . $redirectLocation);

                    // Extract new cookies from auth response
                    $authCookies = $this->extractCookies($authResponse);
                    $this->session = array_merge($cookies, $authCookies);
                    Log::debug("Updated session cookies: " . json_encode(array_keys($this->session)));

                    // Follow the redirect to complete authentication
                    $redirectUrl = $redirectLocation;
                    if (!str_starts_with($redirectLocation, 'http')) {
                        $redirectUrl = 'https://apps.edustar.vic.edu.au' . $redirectLocation;
                    }

                    $redirectHeaders = array_merge($this->headers, [
                        'Referer' => 'https://apps.edustar.vic.edu.au/my.policy',
                        'Cookie' => $this->formatCookies($this->session)
                    ]);

                    Log::info("Following redirect to: " . $redirectUrl);
                    $redirectResponse = Http::withHeaders($redirectHeaders)
                        ->withOptions([
                            'allow_redirects' => ['max' => 5],
                            'timeout' => 30,
                        ])
                        ->get($redirectUrl);

                    if ($redirectResponse->successful()) {
                        // Extract any additional cookies from the redirect
                        $finalCookies = $this->extractCookies($redirectResponse);
                        $this->session = array_merge($this->session, $finalCookies);

                        Log::info("Authentication successful, testing connection...");

                        // Test the connection
                        $connectionTest = $this->testConnection();
                        if (!$connectionTest['success']) {
                            Log::warning("Connection test failed, but auth seemed successful");
                        }

                        $connectionInfo = [
                            'connected' => true,
                            'logged_in_as' => 'Authenticated User',
                            'user_details' => [],
                            'schools' => 1
                        ];

                        $this->connection = $connectionInfo;

                        Log::info("Successfully connected");
                        $success = true;
                        return $connectionInfo;
                    } else {
                        throw new Exception("Failed to follow redirect: " . $redirectResponse->status());
                    }
                } else {
                    // Check response content for error messages
                    $authContent = $authResponse->body();
                    if (strpos($authContent, 'Invalid') !== false ||
                        strpos($authContent, 'denied') !== false ||
                        strpos($authContent, 'error') !== false) {
                        throw new Exception("Authentication failed: Invalid credentials");
                    } else {
                        throw new Exception("Unexpected auth response: " . $authResponse->status());
                    }
                }

            } catch (Exception $e) {
                Log::error("Attempt {$attempt} failed: " . $e->getMessage());

                if ($attempt < $this->maxAttempts) {
                    Log::info("Waiting before retry...");
                    sleep(10);
                } else {
                    Log::error("All connection attempts failed.");
                    throw new Exception("All connection attempts failed: " . $e->getMessage());
                }
            }
        }

        throw new Exception("Connection failed after {$this->maxAttempts} attempts");
    }

    /**
     * Test the connection
     */
    private function testConnection(): array
    {
        try {
            $headers = array_merge($this->headers, [
                'Cookie' => $this->formatCookies($this->session)
            ]);

            $response = Http::withHeaders($headers)
                ->withOptions(['timeout' => 30])
                ->get('https://apps.edustar.vic.edu.au/edustarmc/');

            if ($response->successful()) {
                $content = $response->body();
                if (strpos($content, 'login') === false || strpos($content, 'edustarmc') !== false) {
                    return [
                        'success' => true,
                        'getUser' => ['User' => ['_displayName' => 'Connected User']]
                    ];
                }
            }

            return [
                'success' => false,
                'error' => 'Connection test failed: ' . $response->status()
            ];
        } catch (Exception $e) {
            return [
                'success' => false,
                'error' => $e->getMessage()
            ];
        }
    }

    /**
     * Extract cookies from HTTP response
     */
    private function extractCookies(Response $response): array
    {
        $cookies = [];

        // Get all Set-Cookie headers
        $allHeaders = $response->headers();

        foreach ($allHeaders as $name => $values) {
            if (strtolower($name) === 'set-cookie') {
                $cookieHeaders = is_array($values) ? $values : [$values];

                foreach ($cookieHeaders as $cookieHeader) {
                    if (preg_match('/^([^=]+)=([^;]+)/', $cookieHeader, $matches)) {
                        $cookies[trim($matches[1])] = trim($matches[2]);
                    }
                }
                break;
            }
        }

        Log::debug("Extracted cookies: " . json_encode(array_keys($cookies)));
        return $cookies;
    }

    /**
     * Format cookies for HTTP header
     */
    private function formatCookies(?array $cookies): string
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

    /**
     * Get current connection info
     */
    public function getConnection(): ?array
    {
        return $this->connection;
    }

    /**
     * Check if currently connected
     */
    public function isConnected(): bool
    {
        return $this->connection && $this->connection['connected'];
    }

    /**
     * Make authenticated API call
     */
    public function makeApiCall(string $endpoint, string $method = 'GET', array $data = []): Response
    {
        if (!$this->isConnected()) {
            throw new Exception('Not connected to eduSTAR. Please authenticate first.');
        }

        $headers = array_merge($this->headers, [
            'Cookie' => $this->formatCookies($this->session),
            'Referer' => 'https://apps.edustar.vic.edu.au/edustarmc/'
        ]);

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
