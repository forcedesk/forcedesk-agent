<?php

namespace App\Console\Commands\Services;

use App\Helper\AgentConnectivityHelper;
use App\Models\EdupassAccounts;
use GuzzleHttp\Client;
use GuzzleHttp\Cookie\CookieJar;
use GuzzleHttp\Exception\GuzzleException;
use Illuminate\Console\Command;
use Illuminate\Http\Client\ConnectionException;
use Illuminate\Support\Facades\Http;
use DOMDocument;
use DOMXPath;

class EdustarService extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:edustar-service';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Performs a synchronization of student data from the Edustar Management Console.';

    /**
     * @var CookieJar
     */
    private $cookieJar;

    /**
     * @var Client
     */
    private $client;

    /**
     * Create a new command instance.
     *
     * @return void
     */
    public function __construct()
    {
        parent::__construct();
        $this->cookieJar = new CookieJar();
        $this->client = new Client([
            'cookies' => $this->cookieJar,
            'allow_redirects' => true,
            'timeout' => 30,
        ]);
    }

    private function generateRandomWord(): string
    {
        $phrases = ['wild', 'wood', 'frog', 'beast', 'lion', 'zebra', 'marmoset', 'unicorn', 'rabbit', 'bear', 'float', 'berry', 'puppy', 'cat', 'horse', 'river', 'ocean', 'skies', 'sky', 'golden', 'pear', 'apple', 'raspberry', 'watermelon', 'kiwi', 'basket', 'soccer', 'football', 'jazz', 'fair', 'field', 'insect', 'stick', 'jump', 'bridge', 'log', 'abacus', 'tinsel', 'coat', 'door', 'window', 'free', 'peach', 'cress', 'creek', 'croak', 'crest', 'dino', 'rascal', 'clifford', 'franklin', 'zelda', 'link', 'mario', 'bowser'];

        return $phrases[array_rand($phrases)];
    }

    /**
     * Perform form-based login to EduSTAR
     *
     * @return bool
     */
    private function performLogin(): bool
    {
        try {
            // Get the login page
            $loginUrl = 'https://apps.edustar.vic.edu.au/edustarmc/login'; // Adjust this URL as needed

            $this->info('Fetching login page...');
            $loginPageResponse = $this->client->get($loginUrl);

            // Parse the login form to extract hidden fields (CSRF tokens, etc.)
            $dom = new DOMDocument();
            @$dom->loadHTML($loginPageResponse->getBody());
            $xpath = new DOMXPath($dom);

            $formData = [];

            // Find the login form and extract hidden inputs
            $forms = $xpath->query('//form');
            foreach ($forms as $form) {
                $inputs = $xpath->query('.//input[@type="hidden"]', $form);
                foreach ($inputs as $input) {
                    $name = $input->getAttribute('name');
                    $value = $input->getAttribute('value');
                    if ($name) {
                        $formData[$name] = $value;
                    }
                }

                // Get the form action URL if available
                $action = $form->getAttribute('action');
                if ($action && !str_starts_with($action, 'http')) {
                    $loginUrl = 'https://apps.edustar.vic.edu.au' . $action;
                }
            }

            // Add credentials to form data
            // Note: You may need to adjust these field names based on the actual form
            $formData['username'] = config('agentconfig.emc.emc_username');
            $formData['password'] = config('agentconfig.emc.emc_password');

            // Common alternative field names you might need to try:
            // $formData['email'] = config('agentconfig.emc.emc_username');
            // $formData['user'] = config('agentconfig.emc.emc_username');
            // $formData['login'] = config('agentconfig.emc.emc_username');

            $this->info('Submitting login form...');

            // Submit the login form
            $response = $this->client->post($loginUrl, [
                'form_params' => $formData,
                'headers' => [
                    'Content-Type' => 'application/x-www-form-urlencoded',
                    'Referer' => $loginUrl,
                    'User-Agent' => 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36',
                ]
            ]);

            // Check if login was successful
            $responseBody = $response->getBody()->getContents();

            // You may need to adjust these success indicators based on the actual response
            if (strpos($responseBody, 'dashboard') !== false ||
                strpos($responseBody, 'logout') !== false ||
                $response->getStatusCode() === 200) {

                $this->info('Login successful!');
                return true;
            } else {
                $this->error('Login failed - invalid credentials or form structure changed');
                return false;
            }

        } catch (\Exception $e) {
            $this->error('Login failed: ' . $e->getMessage());
            return false;
        }
    }

    /**
     * Reset student password using authenticated session
     */
    private function resetStudentPassword(string $dn, string $newPassword): bool
    {
        try {
            $response = $this->client->post('https://apps.edustar.vic.edu.au/edustarmc/api/MC/ResetStudentPwd', [
                'form_params' => [
                    'schoolId' => config('agentconfig.emc.emc_school_code'),
                    'dn' => $dn,
                    'newPass' => $newPassword,
                ],
                'headers' => [
                    'Content-Type' => 'application/x-www-form-urlencoded',
                ]
            ]);

            return $response->getStatusCode() === 200;
        } catch (\Exception $e) {
            \Log::error('Could not reset student password: ' . $e->getMessage());
            return false;
        }
    }

    /**
     * Execute the console command.
     *
     * @return int
     */
    public function handle()
    {
        $test = AgentConnectivityHelper::testConnectivity();

        if(!$test) {
            \Log::error('Could not connect to the SchoolDesk instance.');
            $this->error('Connectivity failed to the SchoolDesk instance. Bailing out');
            return false;
        }

        $importcount = 0;
        $deletecount = 0;
        $accounts = [];
        $payload = [];

        if (empty(config('agentconfig.emc.emc_username')) ||
            empty(config('agentconfig.emc.emc_password')) ||
            empty(config('agentconfig.emc.emc_school_code'))) {
            $this->error('Missing EMC configuration');
            return false;
        }

        // Perform form-based login first
        if (!$this->performLogin()) {
            $this->error('Could not authenticate with EduSTAR');
            return false;
        }

        // The URL of the EduSTAR MC API endpoint
        $url = 'https://apps.edustar.vic.edu.au/edustarmc/api/MC/GetStudents/'.config('agentconfig.emc.emc_school_code').'/FULL';

        try {
            $this->info('Fetching student data...');

            // Use the authenticated session to get student data
            $response = $this->client->get($url, [
                'headers' => [
                    'Accept' => 'application/json',
                    'Content-Type' => 'application/json',
                ]
            ]);

            $students = json_decode($response->getBody());

            if (!$students) {
                $this->error('No student data received or invalid JSON response');
                return false;
            }

            $this->info('Processing ' . count($students) . ' students...');

            foreach ($students as $item) {
                $edupassaccount = EdupassAccounts::where('login', $item->_login)->first();

                /* Generate a random password for the account */
                $genpassword = ucwords($this->generateRandomWord()).'.'.rand(1000, 9999);

                $data = [
                    'login' => $item->_login,
                    'firstName' => $item->_firstName,
                    'lastName' => $item->_lastName,
                    'password' => $genpassword,
                    'displayName' => $item->_displayName,
                    'student_class' => $item->_class,
                    'ldap_dn' => $item->_dn,
                ];

                if($edupassaccount) {
                    // Update existing account
                    $edupassaccount->firstName = $data['firstName'];
                    $edupassaccount->lastName = $data['lastName'];
                    $edupassaccount->displayName = $data['displayName'];
                    $edupassaccount->student_class = $data['student_class'];
                    $edupassaccount->ldap_dn = $data['ldap_dn'];
                    $edupassaccount->save();
                } else {
                    // Create new account and reset password using authenticated session
                    if (!$this->resetStudentPassword($data['ldap_dn'], $genpassword)) {
                        \Log::error('Could not reset student password for '.$item->_login);
                    }

                    $edupassaccount = new EdupassAccounts;
                    $edupassaccount->login = $data['login'];
                    $edupassaccount->firstName = $data['firstName'];
                    $edupassaccount->lastName = $data['lastName'];
                    $edupassaccount->displayName = $data['displayName'];
                    $edupassaccount->password = $genpassword;
                    $edupassaccount->student_class = $data['student_class'];
                    $edupassaccount->ldap_dn = $data['ldap_dn'];
                    $edupassaccount->save();
                }

                $accounts[] = $data['login'];
                $payload[] = $data;
                $importcount++;
            }

        } catch (\Exception $e) {
            $this->error('Error fetching student data: ' . $e->getMessage());
            return false;
        }

        /* Delete accounts that have been removed */
        $del_accounts = \App\Models\EduPassAccounts::whereNotIn('login', $accounts)->get();

        foreach($del_accounts as $del_account) {
            $del_account->delete();
            $deletecount++;
        }
        /* End Delete Old Accounts */

        // Send data to SchoolDesk
        try {
            $sdclient = new Client([
                'verify' => false,
                'headers' => [
                    'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
                    'Content-Type' => 'application/json',
                    'x-forcedesk-agent' => config('agentconfig.tenant.tenant_uuid'),
                    'x-forcedesk-agentversion' => config('app.agent_version'),
                ]
            ]);

            $this->info('Posting Data to '.config('agentconfig.tenant.tenant_url') . '/api/agent/ingest/edustar-data');

            $response = $sdclient->post(config('agentconfig.tenant.tenant_url') . '/api/agent/ingest/edustar-data', [
                'body' => json_encode($payload),
            ]);

            $status = json_decode($response->getBody(), false);

            if($status->status != 'ok') {
                $this->error('There was an error sending the data to SchoolDesk!');
                $this->error($status->message);
            } else {
                $this->info('Posting the data succeeded!');
                $this->info($status->message);
            }

        } catch (GuzzleException $e) {
            $this->error('Error posting to SchoolDesk: ' . $e->getMessage());
            return false;
        }

        $this->info('Imported '.$importcount.' accounts.');
        $this->info('Deleted '.$deletecount.' accounts.');

        return true;
    }
}
