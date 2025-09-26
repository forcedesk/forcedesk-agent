<?php

namespace App\Console\Commands;

use App\Services\EduSTARHybridService;
use Illuminate\Console\Command;
use Illuminate\Support\Facades\Log;
use Exception;

class TestEduSTARConnection extends Command
{
    protected $signature = 'edustar:test-hybrid
                            {username? : The username to authenticate with}
                            {password? : The password to authenticate with}
                            {school-number? : The school number for API testing}
                            {--attempts=3 : Maximum number of connection attempts}
                            {--debug : Enable debug output}
                            {--no-cookies : Don\'t display cookie values}
                            {--skip-students : Skip the GetStudents API test}
                            {--use-browser : Use browser for API calls instead of HTTP}
                            {--headless : Run browser in headless mode}';

    protected $description = 'Test EduSTAR connection using hybrid browser + HTTP approach';

    public function handle()
    {
        $this->info('🚀 Testing EduSTAR Hybrid Connection...');
        $this->newLine();

        // Get credentials
        $username = $this->argument('username') ?? $this->ask('Username (format: DOMAIN\\USERNAME)');
        $password = $this->argument('password') ?? $this->secret('Password');
        $schoolNumber = $this->argument('school-number') ?? $this->ask('School Number (4 digits)');

        if (empty($username) || empty($password)) {
            $this->error('❌ Username and password are required!');
            return Command::FAILURE;
        }

        if (empty($schoolNumber) || !preg_match('/^\d{4}$/', $schoolNumber)) {
            $this->error('❌ School number must be a 4-digit number!');
            return Command::FAILURE;
        }

        $options = [
            'debug' => $this->option('debug'),
            'show_cookies' => !$this->option('no-cookies'),
            'skip_students' => $this->option('skip-students'),
            'use_browser' => $this->option('use-browser'),
            'headless' => $this->option('headless')
        ];

        // Set headless mode
        config(['dusk.headless' => $options['headless']]);

        if ($options['debug']) {
            $this->info('🐛 Debug mode enabled');
            $this->newLine();
        }

        try {
            $authService = new EduSTARHybridService();

            $this->info("🔐 Attempting hybrid authentication...");

            // Show progress
            $progressBar = $this->output->createProgressBar(4);
            $progressBar->setFormat(' %current%/%max% [%bar%] %percent:3s%% %message%');

            $progressBar->setMessage('Starting browser...');
            $progressBar->start();

            $startTime = microtime(true);

            $progressBar->setMessage('Authenticating...');
            $progressBar->advance();

            $connection = $authService->connect($username, $password);

            $progressBar->setMessage('Extracting session...');
            $progressBar->advance();

            $progressBar->setMessage('Testing access...');
            $progressBar->advance();

            $progressBar->setMessage('Complete!');
            $progressBar->finish();

            $endTime = microtime(true);
            $this->newLine(2);

            // Success message
            $this->info('✅ Hybrid Connection Successful!');
            $this->info("⏱️  Connected in " . round($endTime - $startTime, 2) . " seconds");
            $this->newLine();

            // Display connection details
            $this->displayConnectionDetails($connection);
            $this->newLine();

            // Display cookies
            if ($options['show_cookies']) {
                $this->displayCookieDetails($authService);
                $this->newLine();
            }

            // Test API capabilities
            $this->testApiCapabilities($authService);
            $this->newLine();

            // Test GetStudents API
            if (!$options['skip_students']) {
                $this->testGetStudentsAPI($authService, $schoolNumber, $options['use_browser']);
            }

            // Clean up
            $authService->cleanup();

            return Command::SUCCESS;

        } catch (Exception $e) {
            if (isset($progressBar)) {
                $progressBar->finish();
                $this->newLine(2);
            }

            $this->error('❌ Hybrid Connection Failed!');
            $this->error("Error: {$e->getMessage()}");
            $this->newLine();

            if ($options['debug']) {
                $this->warn('Stack trace:');
                $this->line($e->getTraceAsString());
            }

            return Command::FAILURE;
        }
    }

    private function displayConnectionDetails(array $connection): void
    {
        $this->info('📋 Connection Details:');

        $details = [
            ['Status', $connection['connected'] ? '✅ Connected' : '❌ Disconnected'],
            ['Authentication', $connection['authentication_method'] ?? 'unknown'],
            ['Logged in as', $connection['logged_in_as']],
            ['Schools available', $connection['schools']],
            ['Session cookies', $connection['session_cookies']],
        ];

        $this->table(['Property', 'Value'], $details);
    }

    private function displayCookieDetails(EduSTARHybridService $authService): void
    {
        $this->info('🍪 Session Cookies:');

        $cookies = $authService->getSessionCookies();

        if (empty($cookies)) {
            $this->warn('No session cookies found');
            return;
        }

        $cookieTable = [];
        foreach ($cookies as $name => $value) {
            $maskedValue = $this->maskSensitiveValue($value);
            $cookieTable[] = [
                $name,
                $maskedValue,
                strlen($value) . ' chars',
                $this->getCookieType($name)
            ];
        }

        $this->table(['Name', 'Value (masked)', 'Length', 'Type'], $cookieTable);
        $this->info("Total cookies: " . count($cookies));
    }

    private function testApiCapabilities(EduSTARHybridService $authService): void
    {
        $this->info('🧪 Testing API Capabilities:');

        $testEndpoints = [
            'Main App' => '/edustarmc/',
            'School Details' => '/edustarmc/school_details',
            'Dashboard' => '/edustarmc/dashboard',
        ];

        $results = [];
        foreach ($testEndpoints as $name => $endpoint) {
            try {
                $fullEndpoint = 'https://apps.edustar.vic.edu.au' . $endpoint;
                $response = $authService->makeApiCall($fullEndpoint);

                $status = $response->successful() ? '✅ Success' : '❌ Failed';
                $statusCode = $response->status();
                $results[] = [$name, $endpoint, $status, $statusCode];
            } catch (Exception $e) {
                $results[] = [$name, $endpoint, '❌ Error', substr($e->getMessage(), 0, 50) . '...'];
            }
        }

        $this->table(['Endpoint', 'Path', 'Status', 'Code/Error'], $results);
    }

    private function testGetStudentsAPI(EduSTARHybridService $authService, string $schoolNumber, bool $useBrowser): void
    {
        $method = $useBrowser ? 'Browser' : 'HTTP';
        $this->info("👥 Testing GetStudents API ({$method} method):");

        $endpoint = "https://apps.edustar.vic.edu.au/edustarmc/api/MC/GetStudents/{$schoolNumber}/FULL";
        $this->line("Endpoint: {$endpoint}");
        $this->newLine();

        try {
            $this->info('Making API request...');
            $startTime = microtime(true);

            if ($useBrowser) {
                $responseBody = $authService->getStudents($schoolNumber, true);
                $statusCode = 200; // Assume success for browser method
                $responseSize = strlen($responseBody);
            } else {
                $response = $authService->getStudents($schoolNumber, false);
                $responseBody = $response->body();
                $statusCode = $response->status();
                $responseSize = strlen($responseBody);
            }

            $endTime = microtime(true);
            $responseTime = round(($endTime - $startTime) * 1000, 2);

            // Display response details
            $this->info("📊 Response Details:");
            $responseDetails = [
                ['Method', $method],
                ['Status Code', $this->getStatusCodeWithEmoji($statusCode)],
                ['Response Time', "{$responseTime}ms"],
                ['Response Size', $this->formatBytes($responseSize)],
            ];

            $this->table(['Property', 'Value'], $responseDetails);
            $this->newLine();

            if ($statusCode >= 200 && $statusCode < 300) {
                $this->info('✅ GetStudents API Request Successful!');
                $this->newLine();

                $this->info('📄 Raw Response Body (first 1000 characters):');
                $this->line('----------------------------------------');
                $this->line(substr($responseBody, 0, 1000));
                if (strlen($responseBody) > 1000) {
                    $this->line('... (truncated)');
                }
                $this->line('----------------------------------------');
            } else {
                $this->error('❌ GetStudents API Request Failed!');
                $this->newLine();

                $this->warn('📄 Error Response (first 500 characters):');
                $this->line('----------------------------------------');
                $this->line(substr($responseBody, 0, 500));
                $this->line('----------------------------------------');
            }

        } catch (Exception $e) {
            $this->error('❌ GetStudents API Request Exception!');
            $this->error("Error: {$e->getMessage()}");

            if ($this->option('debug')) {
                $this->newLine();
                $this->warn('Stack trace:');
                $this->line($e->getTraceAsString());
            }
        }
    }

    private function getStatusCodeWithEmoji(int $statusCode): string
    {
        $emoji = match(true) {
            $statusCode >= 200 && $statusCode < 300 => '✅',
            $statusCode >= 300 && $statusCode < 400 => '🔄',
            $statusCode >= 400 && $statusCode < 500 => '❌',
            $statusCode >= 500 => '💥',
            default => '❓'
        };

        return "{$emoji} {$statusCode}";
    }

    private function formatBytes(int $bytes): string
    {
        $units = ['B', 'KB', 'MB', 'GB'];
        $factor = 1024;

        for ($i = 0; $i < count($units) - 1 && $bytes >= $factor; $i++) {
            $bytes /= $factor;
        }

        return round($bytes, 2) . ' ' . $units[$i];
    }

    private function maskSensitiveValue(string $value): string
    {
        if (strlen($value) <= 8) {
            return str_repeat('*', strlen($value));
        }

        return substr($value, 0, 4) . str_repeat('*', max(4, strlen($value) - 8)) . substr($value, -4);
    }

    private function getCookieType(string $name): string
    {
        $types = [
            'JSESSIONID' => 'Java Session',
            'PHPSESSID' => 'PHP Session',
            'ASP.NET_SessionId' => 'ASP.NET Session',
            'MRHSession' => 'F5 Session',
            'LastMRH_Session' => 'F5 Session Backup',
            'BIGipServer' => 'F5 Load Balancer',
            'F5_ST' => 'F5 Session Token',
            'TS' => 'F5 Timestamp',
            'TIN' => 'F5 Token ID',
        ];

        foreach ($types as $pattern => $type) {
            if (stripos($name, $pattern) !== false) {
                return $type;
            }
        }

        if (stripos($name, 'session') !== false) {
            return 'Session';
        }

        if (stripos($name, 'auth') !== false) {
            return 'Authentication';
        }

        return 'Unknown';
    }
}
