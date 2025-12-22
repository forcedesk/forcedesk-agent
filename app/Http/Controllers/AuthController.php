<?php

namespace App\Http\Controllers;

use Illuminate\Http\Request;
use Illuminate\Support\Facades\Log;
use Inertia\Inertia;
use Inertia\Response;

class AuthController extends Controller
{
    /**
     * Show the login form
     */
    public function showLogin(): Response
    {
        return Inertia::render('Auth/Login');
    }

    /**
     * Handle login attempt
     */
    public function login(Request $request)
    {
        $request->validate([
            'password' => 'required|string',
        ]);

        $correctPassword = env('AGENT_ADMIN_PASSWORD');

        if (empty($correctPassword)) {
            Log::warning('Agent login attempted but AGENT_ADMIN_PASSWORD is not configured', [
                'ip' => $request->ip(),
                'user_agent' => $request->userAgent(),
            ]);

            return back()->withErrors([
                'password' => 'Admin password not configured. Please set AGENT_ADMIN_PASSWORD in your .env file.',
            ]);
        }

        if ($request->password === $correctPassword) {
            Log::info('Agent login successful', [
                'ip' => $request->ip(),
                'user_agent' => $request->userAgent(),
            ]);

            session(['agent_authenticated' => true]);
            return redirect()->route('agent-settings.index');
        }

        // Failed login attempt - log it
        Log::warning('Failed agent login attempt', [
            'ip' => $request->ip(),
            'user_agent' => $request->userAgent(),
            'timestamp' => now()->toDateTimeString(),
        ]);

        return back()->withErrors([
            'password' => 'Invalid password.',
        ]);
    }

    /**
     * Handle logout
     */
    public function logout(Request $request)
    {
        Log::info('Agent logout', [
            'ip' => $request->ip(),
            'user_agent' => $request->userAgent(),
        ]);

        session()->forget('agent_authenticated');
        return redirect()->route('agent.login');
    }
}
