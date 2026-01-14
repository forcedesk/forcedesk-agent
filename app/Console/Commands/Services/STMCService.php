<?php

namespace App\Console\Commands\Services;

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
     * @var string Path to cookie file for session persistence
     */
    private string $cookieFile;

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

        // Create a temp file for cookie storage (session persistence)
        $this->cookieFile = tempnam(sys_get_temp_dir(), 'stmc_cookies_');

        try {
            // Step 1: Initial login to STMC
            $this->info('Step 1: Logging in to STMC...');
            $loginResponse = $this->makeRequest('https://stmc.education.vic.gov.au');
            if ($loginResponse === null) {
                $this->error('Failed to login to STMC.');
                return false;
            }
            $this->info('Login successful.');

            // Step 2: Select school from dropdown
            $this->info('Step 2: Selecting school with code: ' . $schoolCode);
            if (!$this->selectSchool($schoolCode, $loginResponse)) {
                $this->error('Failed to select school.');
                return false;
            }
            $this->info('School selected successfully.');

            // Step 3: Navigate to stud_pwd page
            $this->info('Step 3: Navigating to student password page...');
            $studPwdResponse = $this->makeRequest('https://stmc.education.vic.gov.au/stud_pwd');
            if ($studPwdResponse === null) {
                $this->error('Failed to navigate to student password page.');
                return false;
            }
            $this->info('Student password page accessed.');

            // Step 4: Fetch student data from API
            $this->info('Step 4: Fetching student data...');
            $studentData = $this->makeRequest('https://stmc.education.vic.gov.au/api/SchGetStuds?fullProps=true');

            if ($studentData === null) {
                $this->error('Failed to fetch student data.');
                return false;
            }

            // Output the JSON response to console
            $this->info('Student data retrieved successfully:');
            $decoded = json_decode($studentData, true);
            if ($decoded !== null) {
                $this->line(json_encode($decoded, JSON_PRETTY_PRINT));
            } else {
                $this->line($studentData);
            }

            return true;

        } catch (\Exception $e) {
            $this->error('Error: ' . $e->getMessage());
            return false;
        } finally {
            // Clean up cookie file
            if (file_exists($this->cookieFile)) {
                unlink($this->cookieFile);
            }
        }
    }

    /**
     * Make a cURL request with NTLM authentication
     *
     * @param string $url
     * @param string $method
     * @param array $postData
     * @return string|null
     */
    private function makeRequest(string $url, string $method = 'GET', array $postData = []): ?string
    {
        $ch = curl_init();

        $credentials = agent_config('emc.emc_username') . ':' . agent_config('emc.emc_password');

        curl_setopt_array($ch, [
            CURLOPT_URL => $url,
            CURLOPT_RETURNTRANSFER => true,
            CURLOPT_FOLLOWLOCATION => true,
            CURLOPT_SSL_VERIFYPEER => false,
            CURLOPT_SSL_VERIFYHOST => false,
            CURLOPT_HTTPAUTH => CURLAUTH_NTLM,
            CURLOPT_USERPWD => $credentials,
            CURLOPT_UNRESTRICTED_AUTH => true,
            CURLOPT_COOKIEJAR => $this->cookieFile,
            CURLOPT_COOKIEFILE => $this->cookieFile,
        ]);

        if ($method === 'POST') {
            curl_setopt($ch, CURLOPT_POST, true);
            curl_setopt($ch, CURLOPT_POSTFIELDS, http_build_query($postData));
        }

        $response = curl_exec($ch);
        $httpCode = curl_getinfo($ch, CURLINFO_HTTP_CODE);
        $error = curl_error($ch);
        $errno = curl_errno($ch);

        curl_close($ch);

        if ($errno) {
            $this->warn('cURL error: ' . $error);
            return null;
        }

        if ($httpCode !== 200) {
            $this->warn('HTTP error: ' . $httpCode . ' for URL: ' . $url);
            return null;
        }

        return $response;
    }

    /**
     * Select school from the dropdown based on school code
     *
     * @param string $schoolCode
     * @param string $html
     * @return bool
     */
    private function selectSchool(string $schoolCode, string $html): bool
    {
        // Look for the school in the dropdown options (format: "1234 - Some School")
        if (preg_match('/<option[^>]*value="([^"]*)"[^>]*>[^<]*' . preg_quote($schoolCode, '/') . '[^<]*<\/option>/i', $html, $matches)) {
            $schoolValue = $matches[1];
            $this->info('Found school value: ' . $schoolValue);

            // Submit the school selection
            $response = $this->makeRequest('https://stmc.education.vic.gov.au', 'POST', [
                'school' => $schoolValue,
            ]);

            return $response !== null;
        }

        // Alternative: Try selecting by posting the school code directly
        $this->info('School not found in dropdown, trying direct code: ' . $schoolCode);
        $response = $this->makeRequest('https://stmc.education.vic.gov.au', 'POST', [
            'school' => $schoolCode,
        ]);

        return $response !== null;
    }
}
