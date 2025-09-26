<?php

namespace App\Services;

use Illuminate\Http\Client\PendingRequest;
use Illuminate\Http\Client\Response;
use Illuminate\Support\Facades\Http;
use Illuminate\Support\Facades\Log;
use GuzzleHttp\Cookie\CookieJar;
use Exception;

class EduStarMCService
{
    private ?PendingRequest $httpClient = null;
    private ?array $connection = null;
    private ?CookieJar $cookieJar = null;
    private array $defaultHeaders;
    private string $username;
    private string $password;

    public function __construct()
    {
        $this->defaultHeaders = [
            'User-Agent' => 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
            'Accept' => 'application/json, text/plain, */*',
            'Accept-Language' => 'en-US,en;q=0.9',
            'Referer' => 'https://apps.edustar.vic.edu.au/edustarmc/',
        ];
    }

    /**
     * Authenticates and creates eduSTAR MC session.
     *
     * @param string $username
     * @param string $password
     * @return array Connection details
     * @throws Exception
     */
    public function connect(string $username, string $password): array
    {
        // Clear any existing session
        $this->connection = null;
        $this->httpClient = null;
        $this->cookieJar = null;
        $this->username = $username;
        $this->password = $password;

        Log::info('Connecting to eduSTAR Management Console...');

        $maxAttempts = 3;
        $attempt = 0;
        $success = false;
        $lastException = null;

        while (!$success && $attempt < $maxAttempts) {
            $attempt++;
            Log::info("Connection attempt {$attempt} of {$maxAttempts}...");

            try {
                // Create cookie jar for session management
                $this->cookieJar = new CookieJar();

                // Create new HTTP client with cookie jar
                $this->httpClient = Http::withOptions([
                    'cookies' => $this->cookieJar,
                ])
                    ->withHeaders([
                        'User-Agent' => $this->defaultHeaders['User-Agent'],
                        'Accept' => 'text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8',
                        'Accept-Language' => 'en-US,en;q=0.5',
                        'Cache-Control' => 'no-cache',
                        'Pragma' => 'no-cache',
                    ])
                    ->timeout(30);

                Log::info('Getting login page...');
                $loginPageResponse = $this->httpClient
                    ->get('https://apps.edustar.vic.edu.au/edustarmc/');

                $this->handleSessionConflict($loginPageResponse);

                $this->authenticateUser();

                Log::info('Testing API access...');
                $connectionTest = $this->testConnection();

                if (!$connectionTest['success']) {
                    throw new Exception("Connection test failed: {$connectionTest['error']}");
                }

                $getUserData = $connectionTest['user_data'];

                $this->connection = [
                    'connected' => true,
                    'logged_in_as' => $getUserData['User']['_displayName'] ?? 'Unknown',
                    'user_details' => $getUserData['User'] ?? [],
                    'schools' => count($getUserData['User']['_schools']['ChildNodes'] ?? []),
                ];

                Log::info("Successfully connected as: {$this->connection['logged_in_as']}");
                Log::info("Access to {$this->connection['schools']} school(s)");

                $success = true;
                return $this->connection;

            } catch (Exception $e) {
                $lastException = $e;
                Log::error("Attempt {$attempt} failed: {$e->getMessage()}");

                if ($attempt < $maxAttempts) {
                    Log::info('Waiting before retry...');
                    sleep(10);
                } else {
                    Log::error('All connection attempts failed. The system may be experiencing issues.');
                    throw new Exception("All connection attempts failed: {$e->getMessage()}", 0, $e);
                }
            }
        }

        throw new Exception('Unexpected error in connection loop', 0, $lastException);
    }

    /**
     * Handle session conflicts by creating a new session if needed.
     *
     * @param Response $loginPageResponse
     * @throws Exception
     */
    private function handleSessionConflict(Response $loginPageResponse): void
    {
        $content = $loginPageResponse->body();

        if (strpos($content, 'Access policy evaluation is already in progress') !== false) {
            Log::info('Detected session conflict, extracting newsession URI...');

            $pattern = '/"newsession",\s*"uri":\s*"([^"]*)"/';
            if (preg_match($pattern, $content, $matches)) {
                $newsessionUri = $matches[1];
                Log::info("Found newsession URI: {$newsessionUri}");

                $newSessionUrl = "https://apps.edustar.vic.edu.au/logon.php3?{$newsessionUri}";
                Log::info("Creating new session via: {$newSessionUrl}");

                // Create new cookie jar and HTTP client for clean session
                $this->cookieJar = new CookieJar();
                $this->httpClient = Http::withOptions([
                    'cookies' => $this->cookieJar,
                ])
                    ->withHeaders($this->defaultHeaders)
                    ->timeout(30);

                $this->httpClient->get($newSessionUrl);

                sleep(5);

                Log::info('New session established, proceeding with authentication...');
            } else {
                Log::warning('Could not extract newsession URI, will try direct authentication...');
            }
        } else {
            Log::info('Login page accessible, proceeding with authentication...');
        }
    }

    /**
     * Authenticate the user with credentials.
     *
     * @throws Exception
     */
    private function authenticateUser(): void
    {
        $authData = [
            'curl' => 'Z2Fedustarmc',
            'username' => $this->username,
            'password' => $this->password,
            'SubmitCreds' => 'Log+in',
        ];

        $authHeaders = [
            'Content-Type' => 'application/x-www-form-urlencoded',
            'Referer' => 'https://apps.edustar.vic.edu.au/edustarmc/',
        ];

        Log::info('Submitting credentials...');
        $authResponse = $this->httpClient
            ->withHeaders($authHeaders)
            ->asForm()
            ->post('https://apps.edustar.vic.edu.au/my.policy', $authData);

        if (!$authResponse->successful()) {
            throw new Exception("Authentication failed with status: {$authResponse->status()}");
        }

        // Update default headers for subsequent API calls
        $this->httpClient = $this->httpClient->withHeaders($this->defaultHeaders);
    }

    /**
     * Test the connection by making a test API call.
     *
     * @return array
     */
    private function testConnection(): array
    {
        try {
            // This would be your actual test endpoint
            // Replace with the actual test endpoint used in Test-eduSTARMCConnection
            $response = $this->httpClient->get('https://apps.edustar.vic.edu.au/edustarmc/api/user');

            if ($response->successful()) {
                return [
                    'success' => true,
                    'user_data' => $response->json(),
                ];
            }

            return [
                'success' => false,
                'error' => "API test failed with status: {$response->status()}",
            ];

        } catch (Exception $e) {
            return [
                'success' => false,
                'error' => $e->getMessage(),
            ];
        }
    }

    /**
     * Check if currently connected.
     *
     * @return bool
     */
    public function isConnected(): bool
    {
        return $this->connection !== null && ($this->connection['connected'] ?? false);
    }

    /**
     * Get connection details.
     *
     * @return array|null
     */
    public function getConnection(): ?array
    {
        return $this->connection;
    }

    /**
     * Get the authenticated HTTP client for making API calls.
     *
     * @return PendingRequest|null
     */
    public function getHttpClient(): ?PendingRequest
    {
        return $this->httpClient;
    }

    /**
     * Make an authenticated API call to eduSTAR MC.
     *
     * @param string $method
     * @param string $endpoint
     * @param array $data
     * @return Response
     * @throws Exception
     */
    public function apiCall(string $method, string $endpoint, array $data = []): Response
    {
        if (!$this->isConnected()) {
            throw new Exception('Not connected to eduSTAR MC. Call connect() first.');
        }

        $url = 'https://apps.edustar.vic.edu.au/edustarmc/api/' . ltrim($endpoint, '/');

        return match (strtoupper($method)) {
            'GET' => $this->httpClient->get($url, $data),
            'POST' => $this->httpClient->post($url, $data),
            'PUT' => $this->httpClient->put($url, $data),
            'PATCH' => $this->httpClient->patch($url, $data),
            'DELETE' => $this->httpClient->delete($url, $data),
            default => throw new Exception("Unsupported HTTP method: {$method}"),
        };
    }

    /**
     * Disconnect and clear session.
     */
    public function disconnect(): void
    {
        $this->connection = null;
        $this->httpClient = null;
        $this->cookieJar = null;
        Log::info('Disconnected from eduSTAR MC');
    }

    /**
     * Get the cookie jar for debugging purposes.
     *
     * @return CookieJar|null
     */
    public function getCookieJar(): ?CookieJar
    {
        return $this->cookieJar;
    }
}
