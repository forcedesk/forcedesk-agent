<?php

namespace App\Console\Commands;

use App\Services\EduSTARAuthService;
use Illuminate\Console\Command;
use Illuminate\Support\Facades\Log;
use Exception;

class TestEduSTARConnection extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'edustar:test-connection
                            {username? : The username to authenticate with}
                            {password? : The password to authenticate with}
                            {school-number? : The school number for API testing}
                            {--attempts=3 : Maximum number of connection attempts}
                            {--debug : Enable debug output}
                            {--no-cookies : Don\'t display cookie values}
                            {--skip-students : Skip the GetStudents API test}';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Test connection to EduSTAR, dump connection details, and test GetStudents API';

    /**
     * Execute the console command.
     */
    public function handle()
    {
        $this->info('🔌 Testing EduSTAR Connection...');
        $this->newLine();

        // Get credentials
        $username = $this->argument('username') ?? $this->ask('Username');
        $password = $this->argument('password') ?? $this->secret('Password');
        $schoolNumber = $this->argument('school-number') ?? $this->ask('School Number');

        if (empty($username) || empty($password)) {
            $this->error('Username and password are required!');
            return Command::FAILURE;
        }

        if (empty($schoolNumber)) {
            $this->error('School number is required!');
            return Command::FAILURE;
        }

        // Validate school number format
        if (!preg_match('/^\d{4}$/', $schoolNumber)) {
            $this->error('School number must be a 4-digit number!');
            return Command::FAILURE;
        }

        $maxAttempts = (int) $this->option('attempts');
        $debug = $this->option('debug');
        $showCookies = !$this->option('no-cookies');
        $skipStudents = $this->option('skip-students');

        // Configure logging for debug mode
        if ($debug) {
            $this->info('Debug mode enabled - check logs for detailed output');
            $this->newLine();
        }

        try {
            // Create service instance
            $authService = new EduSTARAuthService($maxAttempts);

            $this->info("Attempting to connect with {$maxAttempts} max attempts...");

            // Create progress bar
            $progressBar = $this->output->createProgressBar($maxAttempts);
            $progressBar->setFormat(' %current%/%max% [%bar%] %percent:3s%% %message%');
            $progressBar->setMessage('Initializing...');
            $progressBar->start();

            // Attempt connection
            $startTime = microtime(true);
            $connection = $authService->connect($username, $password);
            $endTime = microtime(true);

            $progressBar->finish();
            $this->newLine(2);

            // Display success message
            $this->info('✅ Connection Successful!');
            $this->info("⏱️  Connected in " . round($endTime - $startTime, 2) . " seconds");
            $this->newLine();

            // Display connection details
            $this->displayConnectionDetails($connection);
            $this->newLine();

            // Display session cookies if requested
            if ($showCookies) {
                $this->displayCookieDetails($authService);
                $this->newLine();
            }

            // Test API capabilities
            $this->testApiCapabilities($authService);
            $this->newLine();

            // Test GetStudents API if not skipped
            if (!$skipStudents) {
                $this->testGetStudentsAPI($authService, $schoolNumber);
            }

            return Command::SUCCESS;

        } catch (Exception $e) {
            if (isset($progressBar)) {
                $progressBar->finish();
                $this->newLine(2);
            }

            $this->error('❌ Connection Failed!');
            $this->error("Error: {$e->getMessage()}");
            $this->newLine();

            if ($debug) {
                $this->warn('Stack trace:');
                $this->line($e->getTraceAsString());
            }

            return Command::FAILURE;
        }
    }

    /**
     * Display connection details in a formatted table
     */
    private function displayConnectionDetails(array $connection): void
    {
        $this->info('📋 Connection Details:');

        $details = [
            ['Property', 'Value'],
            ['Status', $connection['connected'] ? '✅ Connected' : '❌ Disconnected'],
            ['Logged in as', $connection['logged_in_as']],
            ['Schools available', $connection['schools']],
        ];

        $this->table(['Property', 'Value'], array_slice($details, 1));

        // Display user details if available
        if (!empty($connection['user_details'])) {
            $this->newLine();
            $this->info('👤 User Details:');

            $userDetails = [];
            $this->flattenArray($connection['user_details'], $userDetails);

            $userTable = [];
            foreach ($userDetails as $key => $value) {
                if (is_scalar($value) && !empty($value)) {
                    $userTable[] = [$key, $this->formatValue($value)];
                }
            }

            if (!empty($userTable)) {
                $this->table(['Field', 'Value'], $userTable);
            } else {
                $this->warn('No detailed user information available');
            }
        }
    }

    /**
     * Display cookie information
     */
    private function displayCookieDetails(EduSTARAuthService $authService): void
    {
        $this->info('🍪 Session Cookies:');

        // Use reflection to access private session property
        $reflection = new \ReflectionClass($authService);
        $sessionProperty = $reflection->getProperty('session');
        $sessionProperty->setAccessible(true);
        $cookies = $sessionProperty->getValue($authService);

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

    /**
     * Test API capabilities
     */
    private function testApiCapabilities(EduSTARAuthService $authService): void
    {
        $this->info('🧪 Testing API Capabilities:');

        $testEndpoints = [
            'Profile' => 'https://apps.edustar.vic.edu.au/edustarmc/api/profile',
            'Dashboard' => 'https://apps.edustar.vic.edu.au/edustarmc/dashboard',
            'Home' => 'https://apps.edustar.vic.edu.au/edustarmc/',
        ];

        $results = [];
        foreach ($testEndpoints as $name => $endpoint) {
            try {
                $response = $authService->makeApiCall($endpoint);
                $status = $response->successful() ? '✅ Success' : '❌ Failed';
                $statusCode = $response->status();
                $results[] = [$name, $endpoint, $status, $statusCode];
            } catch (Exception $e) {
                $results[] = [$name, $endpoint, '❌ Error', $e->getMessage()];
            }
        }

        $this->table(['Endpoint', 'URL', 'Status', 'Code/Error'], $results);
    }

    /**
     * Test GetStudents API endpoint
     */
    private function testGetStudentsAPI(EduSTARAuthService $authService, string $schoolNumber): void
    {
        $this->info('👥 Testing GetStudents API:');

        $endpoint = "https://apps.edustar.vic.edu.au/edustarmc/api/MC/GetStudents/{$schoolNumber}/FULL";
        $this->line("Endpoint: {$endpoint}");
        $this->newLine();

        try {
            $this->info('Making API request...');
            $startTime = microtime(true);

            $response = $authService->makeApiCall($endpoint);

            $endTime = microtime(true);
            $responseTime = round(($endTime - $startTime) * 1000, 2);

            // Display response details
            $statusCode = $response->status();
            $contentType = $response->header('Content-Type') ?? 'unknown';
            $responseSize = strlen($response->body());

            $this->info("📊 Response Details:");
            $responseDetails = [
                ['Property', 'Value'],
                ['Status Code', $this->getStatusCodeWithEmoji($statusCode)],
                ['Content Type', $contentType],
                ['Response Time', "{$responseTime}ms"],
                ['Response Size', $this->formatBytes($responseSize)],
            ];

            $this->table(['Property', 'Value'], array_slice($responseDetails, 1));

            if ($response->successful()) {
                $this->newLine();
                $this->info('✅ GetStudents API Request Successful!');

                // Try to parse JSON response
                try {
                    $data = $response->json();
                    $this->displayStudentsData($data, $schoolNumber);
                } catch (Exception $e) {
                    $this->warn('Response is not valid JSON, displaying raw content preview:');
                    $this->displayRawResponse($response->body());
                }
            } else {
                $this->error('❌ GetStudents API Request Failed!');
                $this->newLine();

                // Show error details
                $this->warn('Error Response:');
                $errorContent = $response->body();

                if (!empty($errorContent)) {
                    try {
                        $errorData = $response->json();
                        $this->displayJsonData($errorData, 'Error Details');
                    } catch (Exception $e) {
                        $this->displayRawResponse($errorContent);
                    }
                } else {
                    $this->line('No error content returned');
                }
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

    /**
     * Display students data in a formatted way
     */
    private function displayStudentsData(array $data, string $schoolNumber): void
    {
        $this->newLine();
        $this->info("📚 Students Data for School {$schoolNumber}:");

        if (empty($data)) {
            $this->warn('No student data returned');
            return;
        }

        // Check if it's an array of students or wrapped data
        $students = $data;
        if (isset($data['students'])) {
            $students = $data['students'];
        } elseif (isset($data['data'])) {
            $students = $data['data'];
        } elseif (isset($data[0]) && is_array($data[0])) {
            $students = $data;
        }

        if (is_array($students) && !empty($students)) {
            $this->info("📈 Summary:");
            $summaryTable = [
                ['Total Students', count($students)],
            ];

            // Analyze student data structure
            if (!empty($students[0])) {
                $firstStudent = $students[0];
                $summaryTable[] = ['Fields per Student', count($firstStudent)];

                // Show available fields
                $this->newLine();
                $this->info("🏷️  Available Student Fields:");
                $fields = array_keys($firstStudent);
                $fieldChunks = array_chunk($fields, 4);
                foreach ($fieldChunks as $chunk) {
                    $this->line('  • ' . implode(', ', $chunk));
                }
            }

            $this->newLine();
            $this->table(['Property', 'Value'], $summaryTable);

            // Display sample students (first 5)
            $this->newLine();
            $this->info("👥 Sample Students (first 5):");

            $sampleStudents = array_slice($students, 0, 5);
            $studentTable = [];

            foreach ($sampleStudents as $index => $student) {
                $displayFields = [];

                // Try common field names
                $possibleNameFields = ['name', 'fullName', 'firstName', 'student_name', 'Name', 'FullName'];
                $possibleIdFields = ['id', 'studentId', 'student_id', 'ID', 'StudentID'];
                $possibleYearFields = ['year', 'yearLevel', 'grade', 'Year', 'YearLevel'];

                $name = $this->findFieldValue($student, $possibleNameFields) ?? 'Unknown';
                $id = $this->findFieldValue($student, $possibleIdFields) ?? 'N/A';
                $year = $this->findFieldValue($student, $possibleYearFields) ?? 'N/A';

                $studentTable[] = [
                    $index + 1,
                    $name,
                    $id,
                    $year
                ];
            }

            $this->table(['#', 'Name', 'ID', 'Year'], $studentTable);

            if (count($students) > 5) {
                $this->info("... and " . (count($students) - 5) . " more students");
            }

        } else {
            $this->displayJsonData($data, 'Raw Response Data');
        }
    }

    /**
     * Find field value from possible field names
     */
    private function findFieldValue(array $data, array $possibleFields): ?string
    {
        foreach ($possibleFields as $field) {
            if (isset($data[$field]) && !empty($data[$field])) {
                return (string) $data[$field];
            }
        }
        return null;
    }

    /**
     * Display JSON data in a formatted table
     */
    private function displayJsonData(array $data, string $title = 'Data'): void
    {
        $this->info("📋 {$title}:");

        $flattened = [];
        $this->flattenArray($data, $flattened);

        $table = [];
        foreach ($flattened as $key => $value) {
            if (is_scalar($value)) {
                $table[] = [$key, $this->formatValue($value)];
            }
        }

        if (!empty($table)) {
            $this->table(['Field', 'Value'], $table);
        } else {
            $this->warn('No displayable data found');
        }
    }

    /**
     * Display raw response content (truncated)
     */
    private function displayRawResponse(string $content, int $maxLength = 1000): void
    {
        if (strlen($content) > $maxLength) {
            $this->line(substr($content, 0, $maxLength) . '...');
            $this->info("(Truncated - full content is " . strlen($content) . " characters)");
        } else {
            $this->line($content);
        }
    }

    /**
     * Get status code with appropriate emoji
     */
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

    /**
     * Format bytes to human readable format
     */
    private function formatBytes(int $bytes): string
    {
        $units = ['B', 'KB', 'MB', 'GB'];
        $factor = 1024;

        for ($i = 0; $i < count($units) - 1 && $bytes >= $factor; $i++) {
            $bytes /= $factor;
        }

        return round($bytes, 2) . ' ' . $units[$i];
    }

    /**
     * Flatten nested array for display
     */
    private function flattenArray(array $array, array &$result, string $prefix = ''): void
    {
        foreach ($array as $key => $value) {
            $newKey = $prefix ? "{$prefix}.{$key}" : $key;

            if (is_array($value)) {
                $this->flattenArray($value, $result, $newKey);
            } else {
                $result[$newKey] = $value;
            }
        }
    }

    /**
     * Format value for display
     */
    private function formatValue($value): string
    {
        if (is_bool($value)) {
            return $value ? 'true' : 'false';
        }

        if (is_null($value)) {
            return 'null';
        }

        return (string) $value;
    }

    /**
     * Mask sensitive values for display
     */
    private function maskSensitiveValue(string $value): string
    {
        if (strlen($value) <= 8) {
            return str_repeat('*', strlen($value));
        }

        return substr($value, 0, 4) . str_repeat('*', max(4, strlen($value) - 8)) . substr($value, -4);
    }

    /**
     * Determine cookie type based on name
     */
    private function getCookieType(string $name): string
    {
        $types = [
            'JSESSIONID' => 'Session',
            'PHPSESSID' => 'PHP Session',
            'ASP.NET_SessionId' => 'ASP.NET Session',
            'MRHSession' => 'F5 Session',
            'BIGipServer' => 'F5 Load Balancer',
            'TS' => 'F5 Timestamp',
            'LastMRH_Session' => 'F5 Session Backup',
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
