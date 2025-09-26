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
    private $baseHeaders;

    public function __construct(int $maxAttempts = 3)
    {
        $this->maxAttempts = $maxAttempts;
        $this->session = [];
        $this->connection = null;
        $this->baseHeaders = [
            'User-Agent' => 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:143.0) Gecko/20100101 Firefox/143.0',
            'Accept-Language' => 'en-US,en;q=0.5',
            'Accept-Encoding' => 'gzip, deflate, br, zstd',
            'Connection' => 'keep-alive',
            'Upgrade-Insecure-Requests' => '1',
        ];
    }

    public function connect(string $username, string $password): array
    {
        $success = false;
        $attempt = 0;

        while (!$success && $attempt < $this->maxAttempts) {
            $attempt++;
            Log::info("Connection attempt {$attempt} of {$this->maxAttempts}...");

            try {
                $this->session = [];

                // Step 1: Initial request to establish session
                Log::info("Step 1: Getting initial session...");
                $this->makeInitialRequest();

                // Step 2: Authenticate
                Log::info("Step 2: Authenticating...");
                $this->authenticate($username, $password);

                // Step 3: Verify we can access the main app
                Log::info("Step 3: Verifying access to main application...");
                $this->verifyAccess();

                $connectionInfo = [
                    'connected' => true,
                    'logged_in_as' => 'Authenticated User',
                    'user_details' => [],
                    'schools' => 1
                ];

                $this->connection = $connectionInfo;
                Log::info("Successfully connected with " . count($this->session) . " session cookies");
                $success = true;
                return $connectionInfo;

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

    private function makeInitialRequest(): void
    {
        $headers = array_merge($this->baseHeaders, [
            'Accept' => 'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
        ]);

        $response = Http::withHeaders($headers)
            ->withOptions(['timeout' => 30, 'verify' => false])
            ->get('https://apps.edustar.vic.edu.au/my.policy');

        if (!$response->successful()) {
            throw new Exception("Failed to get initial page: " . $response->status());
        }

        $this->updateSessionCookies($response);
        Log::debug("Initial cookies: " . json_encode(array_keys($this->session)));
    }

    private function authenticate(string $username, string $password): void
    {
        $headers = array_merge($this->baseHeaders, [
            'Accept' => 'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
            'Content-Type' => 'application/x-www-form-urlencoded',
            'Origin' => 'https://apps.edustar.vic.edu.au',
            'Referer' => 'https://apps.edustar.vic.edu.au/my.policy',
            'Sec-Fetch-Dest' => 'document',
            'Sec-Fetch-Mode' => 'navigate',
            'Sec-Fetch-Site' => 'same-origin',
            'Sec-Fetch-User' => '?1',
            'Priority' => 'u=0, i',
            'Cookie' => $this->formatCookies($this->session)
        ]);

        $authData = [
            'username' => $username,
            'password' => $password
        ];

        Log::debug("Auth request cookies: " . $this->formatCookies($this->session));

        $response = Http::withHeaders($headers)
            ->withOptions([
                'allow_redirects' => false,
                'timeout' => 30,
                'verify' => false
            ])
            ->asForm()
            ->post('https://apps.edustar.vic.edu.au/my.policy', $authData);

        Log::debug("Auth response status: " . $response->status());
        Log::debug("Auth response headers: " . json_encode($response->headers()));

        if ($response->status() !== 302) {
            throw new Exception("Authentication failed - expected 302 redirect, got: " . $response->status());
        }

        $this->updateSessionCookies($response);

        $redirectLocation = $response->header('Location');
        if (!$redirectLocation) {
            throw new Exception("No redirect location in auth response");
        }

        Log::info("Auth successful, redirecting to: " . $redirectLocation);

        // Follow the redirect to complete authentication
        $this->followRedirect($redirectLocation, 'https://apps.edustar.vic.edu.au/my.policy');
    }

    private function followRedirect(string $location, string $referer): Response
    {
        if (!str_starts_with($location, 'http')) {
            $location = 'https://apps.edustar.vic.edu.au' . $location;
        }

        $headers = array_merge($this->baseHeaders, [
            'Accept' => 'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
            'Referer' => $referer,
            'Cookie' => $this->formatCookies($this->session)
        ]);

        Log::debug("Following redirect to: " . $location);
        Log::debug("Redirect cookies: " . $this->formatCookies($this->session));

        $response = Http::withHeaders($headers)
            ->withOptions([
                'allow_redirects' => false,
                'timeout' => 30,
                'verify' => false
            ])
            ->get($location);

        Log::debug("Redirect response status: " . $response->status());

        $this->updateSessionCookies($response);

        // If we get another redirect, follow it
        if ($response->status() >= 300 && $response->status() < 400) {
            $nextLocation = $response->header('Location');
            if ($nextLocation) {
                return $this->followRedirect($nextLocation, $location);
            }
        }

        return $response;
    }

    private function verifyAccess(): void
    {
        $headers = array_merge($this->baseHeaders, [
            'Accept' => 'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
            'Referer' => 'https://apps.edustar.vic.edu.au/my.policy',
            'Cookie' => $this->formatCookies($this->session)
        ]);

        Log::debug("Verifying access with cookies: " . $this->formatCookies($this->session));

        $response = Http::withHeaders($headers)
            ->withOptions([
                'allow_redirects' => false,
                'timeout' => 30,
                'verify' => false
            ])
            ->get('https://apps.edustar.vic.edu.au/edustarmc/');

        Log::debug("Verify access response status: " . $response->status());

        $this->updateSessionCookies($response);

        // Check if we're being redirected back to login
        if ($response->status() >= 300 && $response->status() < 400) {
            $location = $response->header('Location');
            if (str_contains($location, 'my.policy') || str_contains($location, 'login')) {
                throw new Exception("Access verification failed - redirected to login");
            }

            // Follow legitimate redirects
            $this->followRedirect($location, 'https://apps.edustar.vic.edu.au/edustarmc/');
        } else if (!$response->successful()) {
            throw new Exception("Access verification failed: " . $response->status());
        }
    }

    public function makeApiCall(string $endpoint, string $method = 'GET', array $data = []): Response
    {
        if (!$this->isConnected()) {
            throw new Exception('Not connected to eduSTAR. Please authenticate first.');
        }

        Log::info("Making API call to: {$endpoint}");

        // First, make sure we have fresh session by visiting the main app
        $this->refreshSession();

        $headers = array_merge($this->baseHeaders, [
            'Accept' => 'application/json, text/html, application/xhtml+xml, application/xml;q=0.9, */*;q=0.8',
            'Referer' => 'https://apps.edustar.vic.edu.au/edustarmc/',
            'Cookie' => $this->formatCookies($this->session)
        ]);

        Log::debug("API call cookies: " . $this->formatCookies($this->session));

        $response = Http::withHeaders($headers)
            ->withOptions([
                'allow_redirects' => false,
                'timeout' => 30,
                'verify' => false
            ])
            ->get($endpoint);

        Log::debug("API response status: " . $response->status());

        // Check for session expiration
        if ($response->status() >= 300 && $response->status() < 400) {
            $location = $response->header('Location');
            if (str_contains($location, 'my.policy') || str_contains($location, 'login')) {
                throw new Exception('Session expired - redirected to login page');
            }
        }

        $this->updateSessionCookies($response);

        return $response;
    }

    private function refreshSession(): void
    {
        Log::debug("Refreshing session...");

        $headers = array_merge($this->baseHeaders, [
            'Accept' => 'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
            'Cookie' => $this->formatCookies($this->session)
        ]);

        $response = Http::withHeaders($headers)
            ->withOptions([
                'allow_redirects' => false,
                'timeout' => 30,
                'verify' => false
            ])
            ->get('https://apps.edustar.vic.edu.au/edustarmc/');

        $this->updateSessionCookies($response);

        Log::debug("Session refreshed, status: " . $response->status());
    }

    private function updateSessionCookies(Response $response): void
    {
        $allHeaders = $response->headers();

        foreach ($allHeaders as $name => $values) {
            if (strtolower($name) === 'set-cookie') {
                $cookieHeaders = is_array($values) ? $values : [$values];

                foreach ($cookieHeaders as $cookieHeader) {
                    if (preg_match('/^([^=]+)=([^;]+)/', $cookieHeader, $matches)) {
                        $cookieName = trim($matches[1]);
                        $cookieValue = trim($matches[2]);
                        $this->session[$cookieName] = $cookieValue;
                        Log::debug("Updated cookie: {$cookieName}={$cookieValue}");
                    }
                }
                break;
            }
        }
    }

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

    public function getConnection(): ?array
    {
        return $this->connection;
    }

    public function isConnected(): bool
    {
        return $this->connection && $this->connection['connected'];
    }

    public function getSessionCookies(): array
    {
        return $this->session;
    }
}
