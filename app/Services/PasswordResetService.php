<?php

/****************************************************************************
 * SchoolDesk - The School Helpdesk System
 *
 * Copyright © 2019 - Excelion/Samuel Brereton. All Rights Reserved.
 *
 * This file or any other component of SchoolDesk cannot be copied, altered
 * and/or distributed without the express permission of SamueL Brereton.
 *
 * Your use of this software is governed by the SchoolDesk EULA. No warranty
 * is expressed or implied except otherwise laid out in your Support Agreement.
 *
 ***************************************************************************/

namespace App\Services;

use App\Http\Controllers\Controller;
use App\Models\EdupassAccounts;
use App\Models\Students;
use Illuminate\Support\Facades\Http;

class PasswordResetService extends Controller
{

    private function rp1()
    {
        $phrases = [
            'smiling',
            'guilty',
            'federal',
            'minor',
            'heavenly',
            'rare',
            'nice',
            'stale',
            'friendly',
            'unusual',
            'ritzy',
            'flimsy',
            'nippy',
            'sable',
            'daisy',
            'four',
            'afraid',
            'fluffy',
            'giant',
            'somber',
            'drunk',
            'erratic',
            'handsome',
            'workable',
            'worried',
            'unequal',
            'foolish',
            'tall',
            'learn',
            'united',
            'wakeful',
            'damaged',
            'panicky',
            'helpless',
            'familiar',
            'furtive',
            'sad',
            'tiny',
            'nature',
            'fragile',
            'conscious',
            'tiresome',
            'unused',
            'weakly',
            'deadly',
            'elated',
            'tanned',
            'existing',
            'fearless',
            'friendly',
            'unwieldy',
            'average',
            'keen',
            'thankful',
            'parallel',
            'spiteful',
            'abstracted',
            'hellish',
            'unable',
            'vigorous',
            'womanly',
            'cheaply',
            'silly',
            'afraid',
            'orange',
            'impartial',
            'basic',
            'majestic',
            'birdcage',
            'careless',
            'adamant',
            'skillful',
            'sticky',
            'melodic',
            'bright',
            'even',
            'married',
            'energetic',
            'wealthy',
            'truthful',
            'victorious',
            'oceanic',
            'responsible',
            'tasteful',
            'froggy'];

        return $phrases[array_rand($phrases)];
    }


    public function handlePasswordReset($kioskid, $username)
    {

        $dinopassgen = Http::get('https://dinopass.com/password/simple');
        $genpassword = ucfirst($dinopassgen);

        // We need to find the user based on the username provided instead of LDAP DN.
        $studentuser = Students::where('username', $username)
            ->first();

        // Check if there is an Edupass Account.
        $edupassacct = EdupassAccounts::where('login', $username)->first();

        if ($studentuser) {

            $studentuser->unicodepwd = $genpassword;

            try {
                $studentuser->save();

                return [
                    'action' => 'authorized',
                    'password' => $genpassword
                ];

            } catch (\LdapRecord\Exceptions\InsufficientAccessException $ex) {
                $error = $ex->getDetailedError();

                \Log::info($error->getErrorCode());
                \Log::info($error->getErrorMessage());
                \Log::info($error->getDiagnosticMessage());

                return [
                    'action' => 'error'
                ];

            } catch (\LdapRecord\Exceptions\ConstraintException $ex) {
                $error = $ex->getDetailedError();

                \Log::info($error->getErrorCode());
                \Log::info($error->getErrorMessage());
                \Log::info($error->getDiagnosticMessage());

                return [
                    'action' => 'error'
                ];
            } catch (\LdapRecord\LdapRecordException $ex) {

                $error = $ex->getDetailedError();

                \Log::info($error->getErrorCode());
                \Log::info($error->getErrorMessage());
                \Log::info($error->getDiagnosticMessage());

                return [
                    'action' => 'error'
                ];
            }
        } elseif ($edupassacct) {

            // Find the EdustarMC Integration
            $integration = \App\Models\Integrations::where('type', 'edustarmc')->first();

            if (empty(config('agentconfig.edustarmc.emc_username')) || empty(config('agentconfig.edustarmc.emc_password')) || empty(config('agentconfig.edustarmc.emc_schoolcode'))) {

                return [
                    'action' => 'declined',
                    'message' => 'API Error. Contact Vendor.',
                ];

            }

            // If the password has been reset in the past 30 minutes, deny the request.
            if ($edupassacct->updated_at->diffInSeconds(\Carbon\Carbon::now()) <= 1800) {

                return [
                    'action' => 'declined',
                    'message' => 'You have changed your password recently.',
                ];

            }

            try {

                $emcpassword = ucwords(self::rp1()).'.'.rand(1000, 9999);

                $response = Http::withBasicAuth(config('agentconfig.edustarmc.emc_username'), config('agentconfig.edustarmc.emc_password'))
                    ->retry(5, 100)
                    ->post('https://apps.edustar.vic.edu.au/edustarmc/api/MC/ResetStudentPwd', [
                        'schoolId' => config('agentconfig.edustarmc.emc_schoolcode'),
                        'dn' => $edupassacct->ldap_dn,
                        'newPass' => $emcpassword,
                    ]);

                $edupassacct->password = $emcpassword;
                $edupassacct->save();

                return [
                    'action' => 'authorized',
                    'payload' => $emcpassword,
                ];


            } catch (\Exception $e) {

                \Log::info($e->getMessage());

                return [
                    'action' => 'declined',
                    'payload' => 'API Error. Contact Vendor',
                ];

            }

        } else {
            return [
                'action' => 'declined',
                'payload' => 'API Error. Contact Vendor',
            ];
        }

    }
}
