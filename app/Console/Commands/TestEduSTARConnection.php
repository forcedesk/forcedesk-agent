<?php

namespace App\Console\Commands;

use App\Services\EduStarMCService;
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
    protected $signature = 'edustar:test
                            {--username= : Username for authentication}
                            {--password= : Password for authentication}
                            {--interactive : Prompt for credentials interactively}
                            {--api-test : Test API calls after connection}
                            {--debug : Show detailed output}';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Test connection to eduSTAR Management Console';

    /**
     * The EduStarMCService instance.
     *
     * @var EduStarMCService
     */
    protected EduStarMCService $eduStarService;

    /**
     * Create a new command instance.
     *
     * @param EduStarMCService $eduStarService
     */
    public function __construct(EduStarMCService $eduStarService)
    {
        parent::__construct();
        $this->eduStarService = $eduStarService;
    }

    /**
     * Execute the console command.
     *
     * @return int
     */
    public function handle()
    {
        $this->info('🚀 Starting eduSTAR Management Console Connection Test');
        $this->newLine();

        try {
            // Get credentials
            $credentials = $this->getCredentials();
            if (!$credentials) {
                $this->error('❌ No valid credentials provided');
                return Command::FAILURE;
            }

            // Test connection
            $this->testConnection($credentials['username'], $credentials['password']);

            // Test API calls if requested
            if ($this->option('api-test')) {
                $this->testApiCalls();
            }

            $this->newLine();
            $this->info('✅ All tests completed successfully!');
            return Command::SUCCESS;

        } catch (Exception $e) {
            $this->newLine();
            $this->error("❌ Test failed: {$e->getMessage()}");

            if ($this->option('debug')) {
                $this->error("Stack trace: {$e->getTraceAsString()}");
            }

            return Command::FAILURE;
        }
    }

    /**
     * Get credentials from various sources.
     *
     * @return array|null
     */
    protected function getCredentials(): ?array
    {
        $username = null;
        $password = null;

        // Option 1: Interactive mode
        if ($this->option('interactive')) {
            $this->info('🔐 Enter your eduSTAR credentials:');
            $username = $this->ask('Username');
            $password = $this->secret('Password');
        }
        // Option 2: Command line options
        elseif ($this->option('username') && $this->option('password')) {
            $username = $this->option('username');
            $password = $this->option('password');
            $this->info('🔐 Using credentials from command options');
        }
        // Option 3: Environment variables
        elseif (config('edustar.username') && config('edustar.password')) {
            $username = config('edustar.username');
            $password = config('edustar.password');
            $this->info('🔐 Using credentials from configuration');
        }
        // Option 4: Fallback to environment variables directly
        elseif (env('EDUSTAR_USERNAME') && env('EDUSTAR_PASSWORD')) {
            $username = env('EDUSTAR_USERNAME');
            $password = env('EDUSTAR_PASSWORD');
            $this->info('🔐 Using credentials from environment variables');
        }

        if (!$username || !$password) {
            $this->warn('⚠️  No credentials found. Please use one of these methods:');
            $this->line('  1. --interactive flag to enter credentials');
            $this->line('  2. --username and --password options');
            $this->line('  3. Set EDUSTAR_USERNAME and EDUSTAR_PASSWORD in .env');
            $this->line('  4. Configure in config/edustar.php');
            return null;
        }

        return [
            'username' => $username,
            'password' => $password,
        ];
    }

    /**
     * Test the connection to eduSTAR MC.
     *
     * @param string $username
     * @param string $password
     * @throws Exception
     */
    protected function testConnection(string $username, string $password): void
    {
        $this->info('🔗 Testing connection to eduSTAR Management Console...');
        $this->newLine();

        $startTime = microtime(true);

        // Show progress bar for connection attempts
        $progressBar = $this->output->createProgressBar(3);
        $progressBar->setFormat('Connecting... %current%/%max% [%bar%] %message%');
        $progressBar->setMessage('Initializing...');
        $progressBar->start();

        try {
            // Capture log messages if debug mode is on
            if ($this->option('debug')) {
                Log::listen(function ($level, $message, $context) {
                    if (str_contains($message, 'eduSTAR') || str_contains($message, 'Connection')) {
                        $this->line("\n[{$level}] {$message}");
                    }
                });
            }

            $connection = $this->eduStarService->connect($username, $password);
            $progressBar->finish();

            $endTime = microtime(true);
            $duration = round($endTime - $startTime, 2);

            $this->newLine(2);
            $this->info("✅ Successfully connected in {$duration} seconds");
            $this->newLine();

            // Display connection details
            $this->displayConnectionDetails($connection);

        } catch (Exception $e) {
            $progressBar->finish();
            $this->newLine(2);
            throw $e;
        }
    }

    /**
     * Display connection details in a formatted table.
     *
     * @param array $connection
     */
    protected function displayConnectionDetails(array $connection): void
    {
        $this->info('📊 Connection Details:');

        $headers = ['Property', 'Value'];
        $rows = [
            ['Status', $connection['connected'] ? '✅ Connected' : '❌ Not Connected'],
            ['Logged in as', $connection['logged_in_as'] ?? 'Unknown'],
            ['Schools accessible', $connection['schools'] ?? 0],
        ];

        $this->table($headers, $rows);

        if ($this->option('debug') && !empty($connection['user_details'])) {
            $this->newLine();
            $this->info('👤 User Details:');
            $userDetails = $connection['user_details'];

            $userRows = [];
            foreach ($userDetails as $key => $value) {
                if (is_string($value) || is_numeric($value)) {
                    $userRows[] = [$key, $value];
                }
            }

            if (!empty($userRows)) {
                $this->table(['Field', 'Value'], $userRows);
            }
        }
    }

    /**
     * Test various API calls.
     */
    protected function testApiCalls(): void
    {
        $this->newLine();
        $this->info('🧪 Testing API calls...');
        $this->newLine();

        $apiTests = [
            ['method' => 'GET', 'endpoint' => 'user', 'description' => 'Get user information'],
            ['method' => 'GET', 'endpoint' => 'schools', 'description' => 'List schools'],
            ['method' => 'GET', 'endpoint' => 'dashboard', 'description' => 'Get dashboard data'],
        ];

        $results = [];

        foreach ($apiTests as $test) {
            $this->line("Testing: {$test['description']}...");

            try {
                $startTime = microtime(true);
                $response = $this->eduStarService->apiCall(
                    $test['method'],
                    $test['endpoint']
                );
                $endTime = microtime(true);

                $duration = round(($endTime - $startTime) * 1000, 2);
                $status = $response->successful() ? '✅ Success' : '❌ Failed';

                $results[] = [
                    $test['description'],
                    $test['method'] . ' /' . $test['endpoint'],
                    $response->status(),
                    $status,
                    $duration . 'ms'
                ];

                if ($this->option('debug') && $response->successful()) {
                    $data = $response->json();
                    if ($data) {
                        $this->line('Response: ' . json_encode($data, JSON_PRETTY_PRINT));
                    }
                }

            } catch (Exception $e) {
                $results[] = [
                    $test['description'],
                    $test['method'] . ' /' . $test['endpoint'],
                    'Error',
                    '❌ Exception',
                    $e->getMessage()
                ];
            }
        }

        $this->newLine();
        $this->info('📋 API Test Results:');
        $this->table(
            ['Test', 'Endpoint', 'Status Code', 'Result', 'Response Time'],
            $results
        );
    }

    /**
     * Show usage examples.
     */
    public function showUsageExamples(): void
    {
        $this->info('📖 Usage Examples:');
        $this->newLine();

        $examples = [
            'Interactive mode' => 'php artisan edustar:test --interactive',
            'With credentials' => 'php artisan edustar:test --username=john.doe --password=secret',
            'Full test suite' => 'php artisan edustar:test --interactive --api-test --debug',
            'Quick connection test' => 'php artisan edustar:test',
        ];

        foreach ($examples as $description => $command) {
            $this->line("<comment>{$description}:</comment>");
            $this->line("  {$command}");
            $this->newLine();
        }
    }
}
