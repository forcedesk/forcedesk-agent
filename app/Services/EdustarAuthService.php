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
    private $httpClient;

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

        // Create a persistent HTTP client that maintains cookies
        $this->httpClient = Http::withOptions([
            'cookies' => true, // Enable cookie jar
            'timeout' => 30,
            'verify' => false // In case of SSL issues
        ]);
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
                $initialResponse = $this->httpClient
                    ->withHeaders($this->headers)
                    ->get('https://apps.edustar.vic.edu.au/my.policy');

                if (!$initialResponse->successful()) {
                    throw new Exception("Failed to get initial page: " . $initialResponse->status());
                }

                // Extract cookies from initial request
                $cookies = $this->extractCookies($initialResponse);
                $this->session = array_merge($this->session, $cookies);
                Log::debug("Initial cookies: " . json_encode(array_keys($cookies)));

                // Step 2: Prepare authentication data
                $authData = [
                    'username' => $username,
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

                // Add cookies
                if (!empty($this->session)) {
                    $authHeaders['Cookie'] = $this->formatCookies($this->session);
                }

                Log::info("Submitting credentials to my.policy...");
                $authResponse = $this->httpClient
                    ->withHeaders($authHeaders)
                    ->withOptions(['allow_redirects' => false])
                    ->asForm()
                    ->post('https://apps.edustar.vic.edu.au/my.policy', $authData);

                Log::debug("Auth response status: " . $authResponse->status());

                // Handle the authentication response
                if ($authResponse->status() === 302) {
                    $redirectLocation = $authResponse->header('Location');
                    Log::info("Got redirect to: " . $redirectLocation);

                    // Extract new cookies from auth response
                    $authCookies = $this->extractCookies($authResponse);
                    $this->session = array_merge($this->session, $authCookies);

                    // Follow the redirect chain completely
                    $finalResponse = $this->followRedirectChain($redirectLocation);

                    if ($finalResponse) {
                        Log::info("Authentication completed successfully");

                        // Update session with any final cookies
                        $finalCookies = $this->extractCookies($finalResponse);
                        $this->session = array_merge($this->session, $finalCookies);

                        Log::debug("Final session cookies: " . json_encode(array_keys($this->session)));

                        $connectionInfo = [
                            'connected' => true,
                            'logged_in_as' => 'Authenticated User',
                            'user_details' => [],
                            'schools' => 1
                        ];

                        $this->connection = $connectionInfo;
                        $success = true;
                        return $connectionInfo;
                    } else {
                        throw new Exception("Failed to complete authentication redirect chain");
                    }
                } else {
                    throw new Exception("Authentication failed: " . $authResponse->status());
                }

            } catch (Exception $e) {
                Log::error("Attempt {$attempt} failed: " . $e->getMessage());

                if ($attempt < $this->maxAttempts) {
                    Log::info("Waiting before retry...");
                    sleep(10);
                } else {
                    throw new Exception("All connection attempts failed: " . $e->getMessage());
                }
            }
        }

        throw new Exception("Connection failed after {$this->maxAttempts} attempts");
    }

    /**
     * Follow redirect chain and maintain session
     */
    private function followRedirectChain(string $initialLocation, int $maxRedirects = 10): ?Response
    {
        $currentLocation = $initialLocation;
        $redirectCount = 0;

        while ($redirectCount < $maxRedirects) {
            // Make the URL absolute if needed
            if (!str_starts_with($currentLocation, 'http')) {
                $currentLocation = 'https://apps.edustar.vic.edu.au' . $currentLocation;
            }

            Log::info("Following redirect #{$redirectCount} to: {$currentLocation}");

            $redirectHeaders = array_merge($this->headers, [
                'Referer' => 'https://apps.edustar.vic.edu.au/my.policy',
            ]);

            // Add current session cookies
            if (!empty($this->session)) {
                $redirectHeaders['Cookie'] = $this->formatCookies($this->session);
            }

            $response = $this->httpClient
                ->withHeaders($redirectHeaders)
                ->withOptions(['allow_redirects' => false])
                ->get($currentLocation);

            // Extract any new cookies
            $newCookies = $this->extractCookies($response);
            $this->session = array_merge($this->session, $newCookies);

            if ($response->status() >= 300 && $response->status() < 400) {
                // Another redirect
                $nextLocation = $response->header('Location');
                if ($nextLocation) {
                    $currentLocation = $nextLocation;
                    $redirectCount++;
                    continue;
                } else {
                    Log::warning("Redirect response without Location header");
                    return $response;
                }
            } else {
                // Final destination reached
                Log::info("Redirect chain completed with status: " . $response->status());
                return $response;
            }
        }

        Log::warning("Maximum redirects reached");
        return null;
    }

    /**
     * Make authenticated API call with better session handling
     */
    public function makeApiCall(string $endpoint, string $method = 'GET', array $data = []): Response
    {
        if (!$this->isConnected()) {
            throw new Exception('Not connected to eduSTAR. Please authenticate first.');
        }

        Log::info("Making API call to: {$endpoint}");
        Log::debug("Current session cookies: " . json_encode(array_keys($this->session)));

        $headers = array_merge($this->headers, [
            'Referer' => 'https://apps.edustar.vic.edu.au/edustarmc/',
            'Accept' => 'application/json, text/html, */*'
        ]);

        // Always include session cookies
        if (!empty($this->session)) {
            $headers['Cookie'] = $this->formatCookies($this->session);
        }

        Log::debug("Request headers: " . json_encode($headers));

        $response = $this->httpClient
            ->withHeaders($headers)
            ->withOptions(['allow_redirects' => false]) // Handle redirects manually
            ->get($endpoint);

        Log::debug("API response status: " . $response->status());
        Log::debug("API response headers: " . json_encode($response->headers()));

        // If we get a redirect, it might be back to login - check for this
        if ($response->status() >= 300 && $response->status() < 400) {
            $location = $response->header('Location');
            Log::warning("API call redirected to: " . $location);

            if (str_contains($location, 'my.policy') || str_contains($location, 'login')) {
                throw new Exception('Session expired - redirected to login page');
            }
        }

        // Extract any new cookies from the response
        $newCookies = $this->extractCookies($response);
        if (!empty($newCookies)) {
            $this->session = array_merge($this->session, $newCookies);
            Log::debug("Updated session with new cookies: " . json_encode(array_keys($newCookies)));
        }

        return $response;
    }

    /**
     * Extract cookies from HTTP response
     */
    private function extractCookies(Response $response): array
    {
        $cookies = [];
        $allHeaders = $response->headers();

        foreach ($allHeaders as $name => $values) {
            if (strtolower($name) === 'set-cookie') {
                $cookieHeaders = is_array($values) ? $values : [$values];

                foreach ($cookieHeaders as $cookieHeader) {
                    if (preg_match('/^([^=]+)=([^;]+)/', $cookieHeader, $matches)) {
                        $cookieName = trim($matches[1]);
                        $cookieValue = trim($matches[2]);
                        $cookies[$cookieName] = $cookieValue;
                        Log::debug("Extracted cookie: {$cookieName}={$cookieValue}");
                    }
                }
                break;
            }
        }

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
     * Get current session cookies (for debugging)
     */
    public function getSessionCookies(): array
    {
        return $this->session;
    }
}
