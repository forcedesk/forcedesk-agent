<?php

namespace App\Console\Commands\Services;

use GuzzleHttp\Client;
use GuzzleHttp\Cookie\CookieJar;
use GuzzleHttp\Exception\GuzzleException;
use Illuminate\Console\Command;

class STMCService extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:stmc-service';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Performs a synchronization of student data from the STMC (EduSTAR Management Console).';

    /**
     * @var CookieJar
     */
    private CookieJar $cookieJar;

    /**
     * @var Client
     */
    private Client $client;

    /**
     * Create a new command instance.
     *
     * @return void
     */
    public function __construct()
    {
        parent::__construct();
    }

    /**
     * Execute the console command.
     *
     * @return int
     */
    public function handle()
    {
        if (empty(agent_config('emc.emc_username')) || empty(agent_config('emc.emc_password')) || empty(agent_config('emc.emc_school_code'))) {
            $this->error('Missing EMC configuration (username, password, or school code).');
            return false;
        }

        $schoolCode = agent_config('emc.emc_school_code');

        // Initialize cookie jar for session persistence
        $this->cookieJar = new CookieJar();

        // Build credentials in domain\username:password format
        $credentials = agent_config('emc.emc_username') . ':' . agent_config('emc.emc_password');

        // Initialize Guzzle client with cURL options for NTLM auth (matching curl --ntlm behavior)
        $this->client = new Client([
            'verify' => false,
            'cookies' => $this->cookieJar,
            'allow_redirects' => true,
            'curl' => [
                CURLOPT_HTTPAUTH => CURLAUTH_NTLM,
                CURLOPT_USERPWD => $credentials,
                CURLOPT_UNRESTRICTED_AUTH => true,
            ],
        ]);

        try {
            // Step 1: Initial login to STMC
            $this->info('Step 1: Logging in to STMC...');
            if (!$this->performLogin()) {
                $this->error('Failed to login to STMC.');
                return false;
            }
            $this->info('Login successful.');

            // Step 2: Select school from dropdown
            $this->info('Step 2: Selecting school with code: ' . $schoolCode);
            if (!$this->selectSchool($schoolCode)) {
                $this->error('Failed to select school.');
                return false;
            }
            $this->info('School selected successfully.');

            // Step 3: Navigate to stud_pwd page
            $this->info('Step 3: Navigating to student password page...');
            if (!$this->navigateToStudentPasswordPage()) {
                $this->error('Failed to navigate to student password page.');
                return false;
            }
            $this->info('Student password page accessed.');

            // Step 4: Fetch student data from API
            $this->info('Step 4: Fetching student data...');
            $studentData = $this->fetchStudentData();

            if ($studentData === false) {
                $this->error('Failed to fetch student data.');
                return false;
            }

            // Output the JSON response to console
            $this->info('Student data retrieved successfully:');
            $this->line(json_encode($studentData, JSON_PRETTY_PRINT));

            return true;

        } catch (\Exception $e) {
            $this->error('Error: ' . $e->getMessage());
            return false;
        }
    }

    /**
     * Perform initial login to STMC
     *
     * @return bool
     */
    private function performLogin(): bool
    {
        try {
            $response = $this->client->get('https://stmc.education.vic.gov.au');

            return $response->getStatusCode() === 200;
        } catch (GuzzleException $e) {
            $this->warn('Login error: ' . $e->getMessage());
            return false;
        }
    }

    /**
     * Select school from the dropdown based on school code
     *
     * @param string $schoolCode
     * @return bool
     */
    private function selectSchool(string $schoolCode): bool
    {
        try {
            // First, get the page to find the school selection form/dropdown
            $response = $this->client->get('https://stmc.education.vic.gov.au');

            $html = (string) $response->getBody();

            // Look for the school in the dropdown options (format: "1234 - Some School")
            // The pattern matches options where the value starts with the school code
            if (preg_match('/<option[^>]*value="([^"]*' . preg_quote($schoolCode, '/') . '[^"]*)"[^>]*>([^<]*' . preg_quote($schoolCode, '/') . '[^<]*)<\/option>/i', $html, $matches)) {
                $schoolValue = $matches[1];
                $this->info('Found school: ' . $matches[2]);

                // Submit the school selection
                $response = $this->client->post('https://stmc.education.vic.gov.au', [
                    'form_params' => [
                        'school' => $schoolValue,
                    ],
                ]);

                return $response->getStatusCode() === 200;
            }

            // Alternative: Try selecting by posting the school code directly
            $response = $this->client->post('https://stmc.education.vic.gov.au', [
                'form_params' => [
                    'school' => $schoolCode,
                ],
            ]);

            return $response->getStatusCode() === 200;

        } catch (GuzzleException $e) {
            $this->warn('School selection error: ' . $e->getMessage());
            return false;
        }
    }

    /**
     * Navigate to the student password page
     *
     * @return bool
     */
    private function navigateToStudentPasswordPage(): bool
    {
        try {
            $response = $this->client->get('https://stmc.education.vic.gov.au/stud_pwd');

            return $response->getStatusCode() === 200;
        } catch (GuzzleException $e) {
            $this->warn('Student password page error: ' . $e->getMessage());
            return false;
        }
    }

    /**
     * Fetch student data from the API
     *
     * @return array|false
     */
    private function fetchStudentData()
    {
        try {
            $response = $this->client->get('https://stmc.education.vic.gov.au/api/SchGetStuds?fullProps=true');

            if ($response->getStatusCode() === 200) {
                return json_decode($response->getBody(), true);
            }

            return false;
        } catch (GuzzleException $e) {
            $this->warn('Fetch student data error: ' . $e->getMessage());
            return false;
        }
    }
}
